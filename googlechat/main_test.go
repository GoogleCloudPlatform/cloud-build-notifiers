package main

import (
	"testing"
)

func TestWriteMessage(t *testing.T) {
	/*
		TODO: write tests.
		n := new(slackNotifier)
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
	*/
}
