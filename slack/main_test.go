package main

import (
	"context"
	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	"testing"
	"text/template"

	"github.com/google/go-cmp/cmp"
	"github.com/slack-go/slack"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
)

type dummySecretGetter struct{}

func (d *dummySecretGetter) GetSecret(_ context.Context, _ string) (string, error) {
	return "", nil
}

func TestWriteMessage(t *testing.T) {
	n := new(slackNotifier)
	b := &cbpb.Build{
		ProjectId: "my-project-id",
		Id:        "some-build-id",
		Status:    cbpb.Build_SUCCESS,
		LogUrl:    "https://some.example.com/log/url?foo=bar",
	}
	tpl, _ := template.New("slack_message").Parse(fallbackTemplate)
	n.tmpl = tpl

	got, err := n.writeMessage(b)
	if err != nil {
		t.Fatalf("writeMessage failed: %v", err)
	}

	want := &slack.WebhookMessage{
		Attachments: []slack.Attachment{{
			Text:  "Cloud Build (my-project-id, some-build-id): SUCCESS",
			Color: "good",
			Actions: []slack.AttachmentAction{{
				Text: "View Logs",
				Type: "button",
				URL:  "https://some.example.com/log/url?foo=bar&utm_campaign=google-cloud-build-notifiers&utm_medium=chat&utm_source=google-cloud-build",
			}},
		}},
	}

	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("writeMessage got unexpected diff: %s", diff)
	}
}

func TestWriteTplMessage(t *testing.T) {
	n := new(slackNotifier)
	b := &cbpb.Build{
		ProjectId: "my-project-id",
	}

	cfg := &notifiers.Config{
		Spec: &notifiers.Spec{
			Notification: &notifiers.Notification{
				Filter: "build.status == Build.Status.SUCCESS",
				Delivery: map[string]interface{}{
					"template": "custom tpl: {{ .ProjectId }}",
					"webhookUrl": map[interface{}]interface{}{
						"secretRef": "ref",
					},
				},
			},
			Secrets: []*notifiers.Secret{
				{LocalName: "ref", ResourceName: ""},
			},
		},
	}
	err := n.SetUp(context.Background(), cfg, new(dummySecretGetter))
	if err != nil {
		t.Fatalf("failed to setup notifier: %v", err)
	}

	got, err := n.writeMessage(b)
	if err != nil {
		t.Fatalf("writeMessage failed: %v", err)
	}

	if len(got.Attachments) == 0 {
		t.Fatalf("unexpected slack message structure")
	}

	actual := got.Attachments[0].Text
	want := "custom tpl: my-project-id"
	if actual != want {
		t.Errorf("wanted slack message text '%s', got '%s'", want, actual)
	}
}
