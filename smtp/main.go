// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"context"
	"fmt"
	"mime/quotedprintable"
	"net/smtp"
	"strings"
	"text/template"

	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	log "github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
)

const (
	contentType = "text/html"
)

func main() {
	if err := notifiers.Main(new(smtpNotifier)); err != nil {
		log.Fatalf("fatal error: %v", err)
	}
}

type smtpNotifier struct {
	filter notifiers.EventFilter
	tmpl   *template.Template
	mcfg   mailConfig
}

type mailConfig struct {
	server, port, sender, from, password string
	recipients                           []string
}

func (s *smtpNotifier) SetUp(ctx context.Context, cfg *notifiers.Config, sg notifiers.SecretGetter, _ notifiers.BindingResolver) error {
	prd, err := notifiers.MakeCELPredicate(cfg.Spec.Notification.Filter)
	if err != nil {
		return fmt.Errorf("failed to create CELPredicate: %w", err)
	}
	s.filter = prd

	tmpl, err := template.New("email_template").Parse(htmlBody)
	if err != nil {
		return fmt.Errorf("failed to parse HTML email template: %w", err)
	}
	s.tmpl = tmpl

	mcfg, err := getMailConfig(ctx, sg, cfg.Spec)
	if err != nil {
		return fmt.Errorf("failed to construct a mail delivery config: %w", err)
	}
	s.mcfg = mcfg

	return nil
}

func getMailConfig(ctx context.Context, sg notifiers.SecretGetter, spec *notifiers.Spec) (mailConfig, error) {
	delivery := spec.Notification.Delivery

	server, ok := delivery["server"].(string)
	if !ok {
		return mailConfig{}, fmt.Errorf("expected delivery config %v to have string field `server`", delivery)
	}
	port, ok := delivery["port"].(string)
	if !ok {
		return mailConfig{}, fmt.Errorf("expected delivery config %v to have string field `port`", delivery)
	}
	sender, ok := delivery["sender"].(string)
	if !ok {
		return mailConfig{}, fmt.Errorf("expected delivery config %v to have string field `sender`", delivery)
	}

	from, ok := delivery["from"].(string)
	if !ok {
		return mailConfig{}, fmt.Errorf("expected delivery config %v to have string field `from`", delivery)
	}

	ris, ok := delivery["recipients"].([]interface{})
	if !ok {
		return mailConfig{}, fmt.Errorf("expected delivery config %v to have repeated field `recipients`", delivery)
	}

	recipients := make([]string, 0, len(ris))
	for _, ri := range ris {
		r, ok := ri.(string)
		if !ok {
			return mailConfig{}, fmt.Errorf("failed to convert recipient (%v) into a string", ri)
		}
		recipients = append(recipients, r)
	}

	passwordRef, err := notifiers.GetSecretRef(delivery, "password")
	if err != nil {
		return mailConfig{}, fmt.Errorf("failed to get ref for secret field `password`: %w", err)
	}

	passwordResource, err := notifiers.FindSecretResourceName(spec.Secrets, passwordRef)
	if err != nil {
		return mailConfig{}, fmt.Errorf("failed to find Secret resource name for reference %q: %w", passwordRef, err)
	}

	password, err := sg.GetSecret(ctx, passwordResource)
	if err != nil {
		return mailConfig{}, fmt.Errorf("failed to get SMTP password: %w", err)
	}

	return mailConfig{
		server:     server,
		port:       port,
		sender:     sender,
		from:       from,
		password:   password,
		recipients: recipients,
	}, nil
}

func (s *smtpNotifier) SendNotification(ctx context.Context, build *cbpb.Build) error {
	if !s.filter.Apply(ctx, build) {
		log.V(2).Infof("no mail for event:\n%s", proto.MarshalTextString(build))
		return nil
	}

	log.Infof("sending email for (build id = %q, status = %s)", build.GetId(), build.GetStatus())
	return s.sendSMTPNotification(build)
}

func (s *smtpNotifier) sendSMTPNotification(build *cbpb.Build) error {
	email, err := s.buildEmail(build)
	if err != nil {
		log.Warningf("failed to build email: %v", err)
	}

	addr := fmt.Sprintf("%s:%s", s.mcfg.server, s.mcfg.port)
	auth := smtp.PlainAuth("", s.mcfg.sender, s.mcfg.password, s.mcfg.server)

	if err = smtp.SendMail(addr, auth, s.mcfg.from, s.mcfg.recipients, []byte(email)); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	log.V(2).Infoln("email sent successfully")
	return nil
}

func (s *smtpNotifier) buildEmail(build *cbpb.Build) (string, error) {
	logURL, err := notifiers.AddUTMParams(build.LogUrl, notifiers.EmailMedium)
	if err != nil {
		return "", fmt.Errorf("failed to add UTM params: %w", err)
	}
	build.LogUrl = logURL

	body := new(bytes.Buffer)
	if err := s.tmpl.Execute(body, build); err != nil {
		return "", err
	}

	subject := fmt.Sprintf("Cloud Build [%s]: %s", build.ProjectId, build.Id)

	header := make(map[string]string)
	if s.mcfg.from != s.mcfg.sender {
		header["Sender"] = s.mcfg.sender
	}
	header["From"] = s.mcfg.from
	header["To"] = strings.Join(s.mcfg.recipients, ",")
	header["Subject"] = subject
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = fmt.Sprintf(`%s; charset="utf-8"`, contentType)
	header["Content-Transfer-Encoding"] = "quoted-printable"
	header["Content-Disposition"] = "inline"

	var msg string
	for key, value := range header {
		msg += fmt.Sprintf("%s: %s\r\n", key, value)
	}

	encoded := new(bytes.Buffer)
	finalMsg := quotedprintable.NewWriter(encoded)
	finalMsg.Write(body.Bytes())
	if err := finalMsg.Close(); err != nil {
		return "", fmt.Errorf("failed to close MIME writer: %w", err)
	}

	msg += "\r\n" + encoded.String()

	return msg, nil
}

const htmlBody = `<!doctype html>
<html>
<head>
<!-- Compiled and minified CSS -->
<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/materialize/0.97.0/css/materialize.min.css">
<!-- Compiled and minified JavaScript -->
<script src="https://cdnjs.cloudflare.com/ajax/libs/materialize/0.97.0/js/materialize.min.js"></script>
<title>Cloud Build Status Email</title>
</head>
<body>
<div class="container">
<div class="row">
<div class="col s2">&nbsp;</div>
<div class="col s8">
<div class="card-content white-text">
<div class="card-title">{{.ProjectId}}: {{.BuildTriggerId}}</div>
</div>
<div class="card-content white">
<table class="bordered">
  <tbody>
	<tr>
	  <td>Status</td>
	  <td>{{.Status}}</td>
	</tr>
	<tr>
	  <td>Log URL</td>
	  <td><a href="{{.LogUrl}}">Click Here</a></td>
	</tr>
  </tbody>
</table>
</div>
</div>
</div>
<div class="col s2">&nbsp;</div>
</div>
</div>
</html>`
