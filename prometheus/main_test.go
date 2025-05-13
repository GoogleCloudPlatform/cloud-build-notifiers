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
	"testing"
	"time"

	cbpb "cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"
)

type mockSecretGetter struct {
	secret string
}

func (m *mockSecretGetter) GetSecret(ctx context.Context, name string) (string, error) {
	return m.secret, nil
}

type mockBindingResolver struct{}

func (m *mockBindingResolver) Resolve(ctx context.Context, tmpl string, build *cbpb.Build) (map[string]string, error) {
	return nil, nil
}

func TestSetUp(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *notifiers.Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: &notifiers.Config{
				Spec: &notifiers.Spec{
					Notification: &notifiers.Notification{
						Filter: "build.status == Build.Status.SUCCESS",
						Delivery: map[string]interface{}{
							"url": "https://prometheus:9090/api/v1/write",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing url",
			cfg: &notifiers.Config{
				Spec: &notifiers.Spec{
					Notification: &notifiers.Notification{
						Filter:   "build.status == Build.Status.SUCCESS",
						Delivery: map[string]interface{}{},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid url",
			cfg: &notifiers.Config{
				Spec: &notifiers.Spec{
					Notification: &notifiers.Notification{
						Filter: "build.status == Build.Status.SUCCESS",
						Delivery: map[string]interface{}{
							"url": "://invalid",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "with basic auth",
			cfg: &notifiers.Config{
				Spec: &notifiers.Spec{
					Notification: &notifiers.Notification{
						Filter: "build.status == Build.Status.SUCCESS",
						Delivery: map[string]interface{}{
							"url":      "https://prometheus:9090/api/v1/write",
							"username": "prometheus",
							"password": map[string]interface{}{
								"secretRef": "prometheus-password",
							},
						},
					},
					Secrets: []*notifiers.Secret{
						{
							Name:  "prometheus-password",
							Value: "projects/test/secrets/prometheus-password/versions/latest",
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &prometheusNotifier{}
			err := p.SetUp(context.Background(), tt.cfg, "", &mockSecretGetter{secret: "test-password"}, &mockBindingResolver{})
			if (err != nil) != tt.wantErr {
				t.Errorf("SetUp() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCollectMetrics(t *testing.T) {
	now := time.Now()
	build := &cbpb.Build{
		Id:        "test-build",
		ProjectId: "test-project",
		Status:    cbpb.Build_SUCCESS,
		StartTime: &timestamp.Timestamp{
			Seconds: now.Unix(),
		},
		FinishTime: &timestamp.Timestamp{
			Seconds: now.Add(time.Minute).Unix(),
		},
		Substitutions: map[string]string{
			"TRIGGER_NAME": "test-trigger",
			"REPO_NAME":    "test-repo",
			"SHORT_SHA":    "abc123",
			"BRANCH_NAME":  "main",
		},
		Steps: []*cbpb.BuildStep{
			{
				Name: "test-step",
				Id:   "step-1",
				Status: cbpb.Build_SUCCESS,
				Timing: &cbpb.TimeSpan{
					StartTime: &timestamp.Timestamp{
						Seconds: now.Unix(),
					},
					EndTime: &timestamp.Timestamp{
						Seconds: now.Add(30 * time.Second).Unix(),
					},
				},
			},
		},
	}

	p := &prometheusNotifier{}
	metrics := p.collectMetrics(build)

	// Verify metrics
	if len(metrics) != 3 { // build duration + step duration + last run status
		t.Errorf("expected 3 metrics, got %d", len(metrics))
	}

	// Verify build duration metric
	buildDuration := metrics[0]
	if buildDuration.Labels[0].Name != "__name__" || buildDuration.Labels[0].Value != "cloudbuild_build_duration_seconds" {
		t.Errorf("invalid metric name for build duration")
	}
	if buildDuration.Samples[0].Value != 60.0 { // 1 minute
		t.Errorf("expected build duration of 60 seconds, got %f", buildDuration.Samples[0].Value)
	}

	// Verify step duration metric
	stepDuration := metrics[1]
	if stepDuration.Labels[0].Name != "__name__" || stepDuration.Labels[0].Value != "cloudbuild_step_duration_seconds" {
		t.Errorf("invalid metric name for step duration")
	}
	if stepDuration.Samples[0].Value != 30.0 { // 30 seconds
		t.Errorf("expected step duration of 30 seconds, got %f", stepDuration.Samples[0].Value)
	}

	// Verify last run status metric
	lastRun := metrics[2]
	if lastRun.Labels[0].Name != "__name__" || lastRun.Labels[0].Value != "cloudbuild_build_last_run_status" {
		t.Errorf("invalid metric name for last run status")
	}
	if lastRun.Samples[0].Value != 1.0 {
		t.Errorf("expected last run status of 1.0, got %f", lastRun.Samples[0].Value)
	}
}

func TestSendNotification(t *testing.T) {
	build := &cbpb.Build{
		Id:        "test-build",
		ProjectId: "test-project",
		Status:    cbpb.Build_SUCCESS,
	}

	tests := []struct {
		name    string
		filter  string
		wantErr bool
	}{
		{
			name:    "filter matches",
			filter:  "build.status == Build.Status.SUCCESS",
			wantErr: false,
		},
		{
			name:    "filter does not match",
			filter:  "build.status == Build.Status.FAILURE",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &prometheusNotifier{}
			cfg := &notifiers.Config{
				Spec: &notifiers.Spec{
					Notification: &notifiers.Notification{
						Filter: tt.filter,
						Delivery: map[string]interface{}{
							"url": "https://prometheus:9090/api/v1/write",
						},
					},
				},
			}

			err := p.SetUp(context.Background(), cfg, "", &mockSecretGetter{}, &mockBindingResolver{})
			if err != nil {
				t.Fatalf("SetUp() error = %v", err)
			}

			err = p.SendNotification(context.Background(), build)
			if (err != nil) != tt.wantErr {
				t.Errorf("SendNotification() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}