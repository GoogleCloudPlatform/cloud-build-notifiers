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
	"context"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
)

func TestSetUp(t *testing.T) {
	const url = "https://some.example.com/notify"

	for _, tc := range []struct {
		name    string
		cfg     *notifiers.Config
		wantErr bool
	}{{
		name: "valid config",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.status == Build.Status.SUCCESS`,
					Delivery: map[string]interface{}{
						"url": url,
					},
				},
			},
		},
	}, {
		name: "missing filter",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Delivery: map[string]interface{}{
						"url": url,
					},
				},
			},
		},
		wantErr: true,
	}, {
		name: "bad filter",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: "blah-#B A D#-",
					Delivery: map[string]interface{}{
						"url": url,
					},
				},
			},
		},
		wantErr: true,
	}, {
		name: "missing delivery url",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.status == Build.Status.SUCCESS`,
					Delivery: map[string]interface{}{
						"foo": "bar",
					},
				},
			},
		},
		wantErr: true,
	}, {
		name: "non-string `url`",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.status == Build.Status.SUCCESS`,
					Delivery: map[string]interface{}{
						"url": 404,
					},
				},
			},
		},
		wantErr: true,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			n := new(httpNotifier)
			err := n.SetUp(context.Background(), tc.cfg, nil, nil)
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

			if n.url != tc.cfg.Spec.Notification.Delivery["url"].(string) {
				t.Errorf("mismatch in post-setup URL: got %q; want %q", n.url, tc.cfg.Spec.Notification.Delivery["url"])
			}
		})
	}
}
