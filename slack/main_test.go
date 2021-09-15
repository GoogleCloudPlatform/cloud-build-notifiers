package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/slack-go/slack"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
)

func TestGetStoragePath(t *testing.T) {
	n := new(slackNotifier)
	result := n.getStoragePath("build-id")
	expected := "messages/build-id"
	if result != expected {
		t.Errorf("Unexpected storage path: %q, expected: %q", result, expected)
	}
	return
}

func TestBuildAttachmentMessageOption(t *testing.T) {
	n := new(slackNotifier)
	b := &cbpb.Build{
		ProjectId: "my-project-id",
		Id:        "some-build-id",
		Status:    cbpb.Build_SUCCESS,
		LogUrl:    "https://some.example.com/log/url?foo=bar",
		SourceProvenance: &cbpb.SourceProvenance{
			ResolvedRepoSource: &cbpb.RepoSource{
				RepoName: "test-repo",
				Revision: &cbpb.RepoSource_BranchName{
					BranchName: "test-branch",
				},
			},
		},
	}

	got := n.buildAttachmentMessageOption(b)

	want := slack.MsgOptionAttachments(
		slack.Attachment{
			Text:  "SUCCESS: :UNKNOWN_REPO: UNKNOWN_REPO (my-project-id) \u003chttps://some.example.com/log/url?foo=bar\u0026utm_campaign=google-cloud-build-notifiers\u0026utm_medium=chat\u0026utm_source=google-cloud-build|View Build\u003e\n*Branch*: UNKNOWN_BRANCH *Author*: UNKNOWN_COMMIT_AUTHOR \n\u003cUNKNOWN_COMMIT_URL|Commit\u003e *UNKNOWN_COMMIT_SHA*: UNKNOWN_COMMIT_MESSAGE",
			Color: "good",
		},
	)

	_, gotValues, err := slack.UnsafeApplyMsgOptions("fake-token", "fake-channel", "https://fake.com/", *got)
	if err != nil {
		t.Errorf("Unable to build message: %w", err)
	}
	_, wantValues, err := slack.UnsafeApplyMsgOptions("fake-token", "fake-channel", "https://fake.com/", want)
	if diff := cmp.Diff(gotValues, wantValues); diff != "" {
		t.Logf("full message: %v", gotValues)
		t.Errorf("writeMessage got unexpected diff: %s", diff)
	}
	return
}
