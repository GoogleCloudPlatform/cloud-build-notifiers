package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/slack-go/slack"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
)

func TestWriteMessage(t *testing.T) {
	n := new(slackNotifier)
	b := &cbpb.Build{
		Name:         "projects/patate/locations/mars/builds/12345",
		Id:           "some-build-id",
		Status:       cbpb.Build_SUCCESS,
		StatusDetail: "this is a detailed success status",
		LogUrl:       "https://some.example.com/log/url?foo=bar",
	}

	got, err := n.writeMessage(b)
	if err != nil {
		t.Fatalf("writeMessage failed: %v", err)
	}

	want := &slack.WebhookMessage{
		Attachments: []slack.Attachment{{
			Text:  "this is a detailed success status for build (12345, some-build-id)",
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
