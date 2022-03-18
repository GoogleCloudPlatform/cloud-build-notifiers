package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	chat "google.golang.org/api/chat/v1"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
)

func TestWriteMessage(t *testing.T) {

	n := new(googlechatNotifier)
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

	want := &chat.Message{
		Cards: []*chat.Card{{
			Header: &chat.CardHeader{
				ImageUrl: "https://www.gstatic.com/images/icons/material/system/2x/check_circle_googgreen_48dp.png",
				Subtitle: "my-project-id",
				Title:    "Build some-bui Status: SUCCESS",
			},
			Sections: []*chat.Section{
				{
					Widgets: []*chat.WidgetMarkup{
						{
							KeyValue: &chat.KeyValue{
								TopLabel: "Duration",
								Content:  "0 min 0 sec",
							},
						},
					},
				},
				{
					Widgets: []*chat.WidgetMarkup{
						{
							Buttons: []*chat.Button{
								{
									TextButton: &chat.TextButton{
										Text: "open logs",
										OnClick: &chat.OnClick{
											OpenLink: &chat.OpenLink{
												Url: "https://some.example.com/log/url?foo=bar&utm_campaign=google-cloud-build-notifiers&utm_medium=chat&utm_source=google-cloud-build",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		}}

	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("writeMessage got unexpected diff: %s", diff)
	}

}
