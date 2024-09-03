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
	"strings"
	"testing"
	"text/template"

	cbpb "cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v2"
)

const password = "rosebud"
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
<div class="card-title">{{.Build.ProjectId}}: {{.Build.BuildTriggerId}}</div>
</div>
<div class="card-content white">
<table class="bordered">
	<tbody>
	<tr>
		<td>Status</td>
		<td>{{.Params.buildStatus}}</td>
	</tr>
	<tr>
		<td>Log URL</td>
		<td><a href="{{.Build.LogUrl}}">Click Here</a></td>
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

const templateSubject = `Build {{.Build.Id}} Status: {{.Build.Status}}`

type fakeSecretGetter struct{}

func (f *fakeSecretGetter) GetSecret(_ context.Context, _ string) (string, error) {
	return password, nil
}

func TestGetMailConfig(t *testing.T) {

	for _, tc := range []struct {
		name       string
		spec       *notifiers.Spec
		wantConfig mailConfig
		wantErr    bool
	}{
		{
			name: "all expected fields present",
			spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `event.status == "SUCCESS"`,
					Delivery: map[string]interface{}{
						"server":     "smtp.example.com",
						"port":       "4040",
						"password":   map[interface{}]interface{}{"secretRef": "my-smtp-password"},
						"sender":     "me@example.com",
						"from":       "another_me@example.com",
						"recipients": []interface{}{"my-cto@example.com", "my-friend@example.com"},
					},
				},
				Secrets: []*notifiers.Secret{{LocalName: "my-smtp-password", ResourceName: "/does/not/matter"}},
			},
			wantConfig: mailConfig{
				server:     "smtp.example.com",
				port:       "4040",
				password:   password,
				sender:     "me@example.com",
				from:       "another_me@example.com",
				recipients: []string{"my-cto@example.com", "my-friend@example.com"},
			},
		}, {
			name: "server is missing",
			spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Delivery: map[string]interface{}{
						"port":       "4040",
						"password":   map[interface{}]interface{}{"secretRef": "my-smtp-password"},
						"sender":     "me@example.com",
						"from":       "another_me@example.com",
						"recipients": []interface{}{"my-cto@example.com", "my-friend@example.com"},
					},
				},
				Secrets: []*notifiers.Secret{{LocalName: "my-smtp-password", ResourceName: "/does/not/matter"}},
			},
			wantErr: true,
		},
		// TODO(ljr): Add more error cases.
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotConfig, err := getMailConfig(context.Background(), new(fakeSecretGetter), tc.spec)
			if err != nil {
				if !tc.wantErr {
					t.Fatalf("unexpected error: %v", err)
				} else {
					t.Logf("got expected error: %v", err)
				}
			}

			if diff := cmp.Diff(tc.wantConfig, gotConfig, cmp.AllowUnexported(mailConfig{})); diff != "" {
				t.Errorf("unexpected diff: %v", diff)
			}
		})
	}
}

func TestCorrectYAMLParseToMailConfig(t *testing.T) {
	const yamlConfig = `
apiVersion: cloud-build-notifiers/v1
kind: SMTPNotifier
metadata:
  name: failed-build-email-notification
spec:
  notification:
    filter: event.buildTriggerStatus == "STATUS_FAILED"
    delivery:
      server: smtp.example.com
      port: '587'
      sender: my-notifier@example.com
      from: my-notifier-from@example.com
      password:
        secretRef: smtp-password
      recipients:
        - some-eng@example.com
        - me@example.com
  secrets:
    - name: smtp-password
      value: projects/some-project/secrets/smtp-notifier-password/versions/latest
 `

	wantMailConfig := mailConfig{
		server:     "smtp.example.com",
		port:       "587",
		password:   password,
		sender:     "my-notifier@example.com",
		from:       "my-notifier-from@example.com",
		recipients: []string{"some-eng@example.com", "me@example.com"},
	}

	cfg := new(notifiers.Config)
	dcd := yaml.NewDecoder(bytes.NewBufferString(yamlConfig))
	dcd.SetStrict(true)
	if err := dcd.Decode(cfg); err != nil {
		t.Fatalf("failed to decode YAML: %v", err)
	}

	gotMailConfig, err := getMailConfig(context.Background(), new(fakeSecretGetter), cfg.Spec)
	if err != nil {
		t.Errorf("getMailConfig failed unexpectedly: %v", err)
	}

	if diff := cmp.Diff(wantMailConfig, gotMailConfig, cmp.AllowUnexported(mailConfig{})); diff != "" {
		t.Errorf("gotMailConfig got unexpected diff: %s", diff)
	}
}

func TestDefaultEmailTemplate(t *testing.T) {
	tmpl, err := template.New("email_template").Parse(htmlBody)
	if err != nil {
		t.Fatalf("template.Parse failed: %v", err)
	}

	build := &cbpb.Build{
		Id:             "some-build-id",
		ProjectId:      "my-project-id",
		BuildTriggerId: "some-trigger-id",
		Status:         cbpb.Build_SUCCESS,
		LogUrl:         "https://some.example.com/log/url",
	}

	view := &notifiers.TemplateView{
		Build: &notifiers.BuildView{
			Build: build,
		},
		Params: map[string]string{"buildStatus": "SUCCESS"},
	}

	body := new(bytes.Buffer)
	if err := tmpl.Execute(body, view); err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	if !strings.Contains(body.String(), `<div class="card-title">my-project-id: some-trigger-id</div>`) {
		t.Error("missing correct .card-title div")
	}

	if !strings.Contains(body.String(), `SUCCESS`) {
		t.Error("missing status")
	}

	if !strings.Contains(body.String(), `<a href="https://some.example.com/log/url">`) {
		t.Error("missing Log URL")
	}
}

func TestSubjectEmailTemplate(t *testing.T) {
	tmpl, err := template.New("subject_template").Parse(templateSubject)
	if err != nil {
			t.Fatalf("failed to parse subject template: %v", err)
	}

	build := &cbpb.Build{
		Id:             "some-build-id",
		ProjectId:      "my-project-id",
		BuildTriggerId: "some-trigger-id",
		Status:         cbpb.Build_SUCCESS,
		LogUrl:         "https://some.example.com/log/url",
	}

	view := &notifiers.TemplateView{
		Build: &notifiers.BuildView{
			Build: build,
		},
		Params: map[string]string{"buildStatus": "SUCCESS"},
	}

	subject := new(bytes.Buffer)
	if err := tmpl.Execute(subject, view); err != nil {
			t.Fatalf("failed to execute subject template: %v", err)
	}

	expectedSubject := "Build some-build-id Status: SUCCESS"
	if subject.String() != expectedSubject {
			t.Errorf("expected subject %q, but got %q", expectedSubject, subject.String())
	}
}
