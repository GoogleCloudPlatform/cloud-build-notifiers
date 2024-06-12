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
	htmlTemplate "html/template"
	textTemplate "text/template"
	"mime/quotedprintable"
	"net/smtp"
	"strings"

	cbpb "cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	log "github.com/golang/glog"
	"google.golang.org/protobuf/encoding/prototext"
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
	filter   notifiers.EventFilter
	htmlTmpl *htmlTemplate.Template
	textTmpl *textTemplate.Template
	mcfg     mailConfig
	br       notifiers.BindingResolver
	tmplView *notifiers.TemplateView
}

type mailConfig struct {
	server, port, sender, from, password, subject string
	recipients                                    []string
}

func (s *smtpNotifier) SetUp(ctx context.Context, cfg *notifiers.Config, cfgTemplate string, sg notifiers.SecretGetter, br notifiers.BindingResolver) error {
	prd, err := notifiers.MakeCELPredicate(cfg.Spec.Notification.Filter)
	if err != nil {
		return fmt.Errorf("failed to create CELPredicate: %w", err)
	}
	s.filter = prd
	htmlTmpl, err := htmlTemplate.New("email_template").Parse(cfgTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse HTML email template: %w", err)
	}
	s.htmlTmpl = htmlTmpl

	if subject, subjectFound := cfg.Spec.Notification.Delivery["subject"]; subjectFound {
		textTmpl, err := textTemplate.New("subject_template").Parse(subject.(string))
		if err != nil {
			return fmt.Errorf("failed to parse TEXT subject template: %w", err)
		}
		s.textTmpl = textTmpl
	}

	mcfg, err := getMailConfig(ctx, sg, cfg.Spec)
	if err != nil {
		return fmt.Errorf("failed to construct a mail delivery config: %w", err)
	}
	s.mcfg = mcfg
	s.br = br
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
		log.V(2).Infof("no mail for event:\n%s", prototext.Format(build))
		return nil
	}
	bindings, err := s.br.Resolve(ctx, nil, build)
	if err != nil {
		log.Errorf("failed to resolve bindings :%v", err)
	}
	s.tmplView = &notifiers.TemplateView{
		Build:  &notifiers.BuildView{Build: build},
		Params: bindings,
	}
	log.Infof("sending email for (build id = %q, status = %s)", build.GetId(), build.GetStatus())
	return s.sendSMTPNotification()
}

func (s *smtpNotifier) sendSMTPNotification() error {
	email, err := s.buildEmail()
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

func (s *smtpNotifier) buildEmail() (string, error) {
	build := s.tmplView.Build
	logURL, err := notifiers.AddUTMParams(s.tmplView.Build.LogUrl, notifiers.EmailMedium)
	if err != nil {
		return "", fmt.Errorf("failed to add UTM params: %w", err)
	}
	build.LogUrl = logURL

	body := new(bytes.Buffer)
	if err := s.htmlTmpl.Execute(body, s.tmplView); err != nil {
		return "", err
	}

	subject := fmt.Sprintf("Cloud Build [%s]: %s", build.ProjectId, build.Id)
	if s.textTmpl != nil {
		subjectTmpl := new(bytes.Buffer)
		if err := s.textTmpl.Execute(subjectTmpl, s.tmplView); err != nil {
			return "", err
		}

		// Escape any string formatter
		subject = strings.Join(strings.Fields(subjectTmpl.String()), " ")
	}

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
