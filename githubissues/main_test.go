// Copyright 2022 Google LLC
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
)

const githubToken = "ghtABC="

type fakeSecretGetter struct{}

func (f *fakeSecretGetter) GetSecret(_ context.Context, _ string) (string, error) {
	return githubToken, nil
}

const issuePayload = `
{
    "title": "Cloud Build [{{.Build.ProjectId}}]: {{.Build.Status}}",
    "body": "Cloud Build {{.Build.ProjectId}} {{.Build.BuildTriggerId}} status: **{{.Build.Status}}**\n\n[View Logs]({{.Build.LogUrl}})"
}`

func TestDefaultIssueTempate(t *testing.T) {

	tmpl, err := template.New("issue_template").Parse(issuePayload)
	if err != nil {
		t.Fatalf("template.Parse failed: %v", err)
	}
	build := &cbpb.Build{
		ProjectId: "my-project-id",
		Id:        "some-build-id",
		Status:    cbpb.Build_SUCCESS,
		LogUrl:    "https://some.example.com/log/url?foo=bar",
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

	if !strings.Contains(body.String(), `SUCCESS`) {
		t.Error("missing status")
	}

}

func TestConfigs(t *testing.T) {
	const repo = "somename/somerepo"
	goodDelivery := map[string]interface{}{
		"githubToken": map[interface{}]interface{}{"secretRef": "mytoken"},
		"githubRepo":  repo,
	}
	goodSecret := []*notifiers.Secret{{LocalName: "mytoken", ResourceName: "mysekrit"}}

	for _, tc := range []struct {
		name    string
		cfg     *notifiers.Config
		wantErr bool
	}{{
		name: "valid config",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter:   `build.status == Build.Status.SUCCESS`,
					Delivery: goodDelivery,
				},
				Secrets: goodSecret,
			},
		},
	}, {
		name: "missing filter",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Delivery: goodDelivery,
				},
				Secrets: goodSecret,
			},
		},
		wantErr: true,
	}, {
		name: "bad filter",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter:   "blah-#B A D#-",
					Delivery: goodDelivery,
				},
				Secrets: goodSecret,
			},
		},
		wantErr: true,
	}, {
		name: "missing delivery repo",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.status == Build.Status.SUCCESS`,
					Delivery: map[string]interface{}{
						"githubToken": map[interface{}]interface{}{"secretRef": "mytoken"},
					},
				},
				Secrets: goodSecret,
			},
		},
		wantErr: true,
	}, {
		name: "missing secret",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.status == Build.Status.SUCCESS`,
					Delivery: map[string]interface{}{
						"githubRepo": repo,
					},
				},
			},
		},
		wantErr: true,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			n := new(githubissuesNotifier)
			err := n.SetUp(context.Background(), tc.cfg, "", new(fakeSecretGetter), nil)
			if err != nil {
				if tc.wantErr {
					t.Logf("got expected error: %v", err)
					return
				}
				t.Fatalf("SetUp(%v) got unexpected error: %v", tc.cfg, err)
			}

			if tc.wantErr {
				t.Error("unexpected success")
			}
		})
	}
}
