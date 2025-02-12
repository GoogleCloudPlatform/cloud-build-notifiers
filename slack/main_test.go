package main

import (
	"testing"
	"text/template"
	"strings"

	cbpb "cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	"github.com/google/go-cmp/cmp"
	"github.com/slack-go/slack"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/protoadapt"
)

func TestWriteMessage(t *testing.T) {
	n := new(slackNotifier)

	rawPubSubMessage := `{
	  	"id": "111222333-4455-6677-8899-fa12345678",
		"status": "SUCCESS",
  		"projectId": "hello-world-123",
		"logUrl": "https://some.example.com/log/url?foo=bar\"",
		"substitutions": {
			"_GOOGLE_FUNCTION_TARGET": "helloHttp",
		}
	}`
	
	uo := protojson.UnmarshalOptions{
		AllowPartial:   true,
		DiscardUnknown: true,
	}
	
	build := new(cbpb.Build)
	bv2 := protoadapt.MessageV2Of(build)
	uo.Unmarshal([]byte(rawPubSubMessage), bv2)
	build = protoadapt.MessageV1Of(bv2).(*cbpb.Build)

	blockKitTemplate := `[
		{
		  "type": "section",
		  "text": {
			"type": "mrkdwn",
			"text": "Build {{.Build.Substitutions._GOOGLE_FUNCTION_TARGET}} Status: {{.Build.Status}}"
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
			"url": "{{replace .Build.LogUrl "\"" "'"}}",
			"action_id": "button-action"
		  }
		}
	  ]`

	tmpl, err := template.New("blockkit_template").Funcs(template.FuncMap{
		"replace": func(s, old, new string) string {
			return strings.ReplaceAll(s, old, new)
		},
	}).Parse(blockKitTemplate)
	if err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	n.tmpl = tmpl
	n.tmplView = &notifiers.TemplateView{Build: &notifiers.BuildView{Build: build}}

	got, err := n.writeMessage()
	if err != nil {
		t.Fatalf("writeMessage failed: %v", err)
	}

	want := &slack.WebhookMessage{
		Attachments: []slack.Attachment{{
			Color: "#22bb33",
			Blocks: slack.Blocks{
				BlockSet: []slack.Block{
					&slack.SectionBlock{
						Type: "section",
						Text: &slack.TextBlockObject{
							Type: "mrkdwn",
							Text: "Build helloHttp Status: SUCCESS",
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
							URL:      "https://some.example.com/log/url?foo=bar'",
							Value:    "click_me_123",
						}},
					},
				},
			},
		}},
	}

	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("writeMessage got unexpected diff: %s", diff)
	}
}
