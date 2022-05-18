package main

import (
	"testing"
	"text/template"

	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	"github.com/google/go-cmp/cmp"
	"github.com/slack-go/slack"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
)

func TestWriteMessage(t *testing.T) {
	n := new(slackNotifier)

	blockKitTemplate := `[
		{
		  "type": "section",
		  "text": {
			"type": "mrkdwn",
			"text": "Build Status: {{.Build.Status}}"
		  }
		},
		{
		  "type": "divider"
		},
		{
		  "type": "section",
		  "text": {
			"type": "mrkdwn",
			"text": "View Build Logs"
		  },
		  "accessory": {
			"type": "button",
			"text": {
			  "type": "plain_text",
			  "text": "Logs"
			},
			"value": "click_me_123",
			"url": "{{.Build.LogUrl}}",
			"action_id": "button-action"
		  }
		}
	  ]`

	tmpl, err := template.New("blockkit_template").Parse(blockKitTemplate)
	if err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}
	n.tmpl = tmpl
	n.tmplView = &notifiers.TemplateView{Build: &notifiers.BuildView{Build: &cbpb.Build{
		ProjectId: "my-project-id",
		Id:        "some-build-id",
		Status:    cbpb.Build_SUCCESS,
		LogUrl:    "https://some.example.com/log/url?foo=bar",
	}}}

	got, err := n.writeMessage()
	if err != nil {
		t.Fatalf("writeMessage failed: %v", err)
	}

	want := &slack.WebhookMessage{
		Attachments: []slack.Attachment{{Color: "good"}},
		Blocks: &slack.Blocks{
			BlockSet: []slack.Block{
				&slack.SectionBlock{
					Type: "section",
					Text: &slack.TextBlockObject{
						Type: "mrkdwn",
						Text: "Build Status: SUCCESS",
					},
				},
				&slack.DividerBlock{
					Type: "divider",
				},
				&slack.SectionBlock{
					Type: "section",
					Text: &slack.TextBlockObject{
						Type: "mrkdwn",
						Text: "View Build Logs",
					},
					Accessory: &slack.Accessory{ButtonElement: &slack.ButtonBlockElement{
						Type:     "button",
						Text:     &slack.TextBlockObject{Type: "plain_text", Text: "Logs"},
						ActionID: "button-action",
						URL:      "https://some.example.com/log/url?foo=bar",
						Value:    "click_me_123",
					}},
				},
			},
		},
	}

	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("writeMessage got unexpected diff: %s", diff)
	}
}
