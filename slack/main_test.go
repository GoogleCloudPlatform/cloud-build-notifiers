package main

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/slack-go/slack"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"

	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
)

type constantSecretGetter string

func (c constantSecretGetter) GetSecret(_ context.Context, _ string) (string, error) {
	return string(c), nil
}

func TestWriteMessage(t *testing.T) {
	tests := []struct {
		name         string
		template     string
		expectedText string
	}{
		{
			name:         "default",
			expectedText: "Cloud Build (my-project-id, some-build-id): SUCCESS",
		},
		{
			name:         "custom message",
			template:     "Hello there {{.Id}}",
			expectedText: "Hello there some-build-id",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			n := new(slackNotifier)
			delivery := map[string]interface{}{
				"webhookUrl": map[interface{}]interface{}{
					"secretRef": "webhook-url",
				},
			}
			if test.template != "" {
				delivery[textTemplateName] = test.template
			}
			config := &notifiers.Config{
				Spec: &notifiers.Spec{
					Notification: &notifiers.Notification{
						Filter:   "true",
						Delivery: delivery,
					},
					Secrets: []*notifiers.Secret{
						{
							LocalName:    "webhook-url",
							ResourceName: "dummy",
						},
					},
				},
			}
			err := n.SetUp(context.Background(), config, constantSecretGetter("hello"), nil)
			if err != nil {
				t.Fatalf("SetUp failed: %v", err)
			}

			b := &cbpb.Build{
				ProjectId: "my-project-id",
				Id:        "some-build-id",
				Status:    cbpb.Build_SUCCESS,
				LogUrl:    "https://some.example.com/log/url?foo=bar",
			}

			got, err := n.writeMessage(b)
			if err != nil {
				t.Fatalf("writeMessage failed: %v", err)
			}

			want := &slack.WebhookMessage{
				Attachments: []slack.Attachment{{
					Text:  test.expectedText,
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
		})
	}
}
