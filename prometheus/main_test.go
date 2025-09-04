// Copyright 2024 Google LLC
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
	"fmt"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
)

const prometheusPasswordResource = "projects/test/secrets/prometheus-password/versions/latest"
const prometheusPassword = "ThisIsStrongPassword"

type fakeSecretGetter struct{}

func (f *fakeSecretGetter) GetSecret(_ context.Context, name string) (string, error) {
	if name != prometheusPasswordResource {
		return "", fmt.Errorf("Unexpected secret %s", name)
	}
	return prometheusPassword, nil
}

func TestSetUp(t *testing.T) {
	const url = "https://prometheus.example.com/api/v1/write"

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
		wantErr: false,
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
	}, {
		name: "invalid url",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.status == Build.Status.SUCCESS`,
					Delivery: map[string]interface{}{
						"url": "://invalid",
					},
				},
			},
		},
		wantErr: true,
	}, {
		name: "with basic auth",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.status == Build.Status.SUCCESS`,
					Delivery: map[string]interface{}{
						"url":      url,
						"username": "username",
						"password": map[interface{}]interface{}{
							"secretRef": "prometheus-password",
						},
					},
				},
				Secrets: []*notifiers.Secret{
					{
						LocalName:    "prometheus-password",
						ResourceName: prometheusPasswordResource,
					},
				},
			},
		},
		wantErr: false,
	}, {
		name: "username without password",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.status == Build.Status.SUCCESS`,
					Delivery: map[string]interface{}{
						"url":      url,
						"username": "username",
					},
				},
			},
		},
		wantErr: true,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			n := new(prometheusNotifier)
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
