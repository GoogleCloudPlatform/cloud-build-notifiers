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
	"time"

	cbpb "cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const prometheusPasswordResource = "projects/test/secrets/prometheus-password/versions/latest"
const prometheusPassword = "michaelact"

type fakeSecretGetter struct{}

func (f *fakeSecretGetter) GetSecret(_ context.Context, name string) (string, error) {
	switch name {
	case prometheusPasswordResource:
		return prometheusPassword, nil
	default:
		return "", fmt.Errorf("Unexpected secret %s", name)
	}
}

// createCompleteBuild creates a build with all required substitutions and fields
func createCompleteBuild(id, projectID string, status cbpb.Build_Status, startTime, finishTime time.Time) *cbpb.Build {
	return &cbpb.Build{
		Id:        id,
		ProjectId: projectID,
		Status:    status,
		StartTime: timestamppb.New(startTime),
		FinishTime: timestamppb.New(finishTime),
		Substitutions: map[string]string{
			"TRIGGER_NAME": "test-trigger",
			"REPO_NAME":    "test-repo",
			"SHORT_SHA":    "abc123",
			"BRANCH_NAME":  "main",
			"TAG_NAME":     "", // Empty for branch builds
		},
		Options: &cbpb.BuildOptions{
			MachineType: cbpb.BuildOptions_E2_HIGHCPU_8,
		},
		Steps: []*cbpb.BuildStep{
			{
				Name:   "test-step-1",
				Id:     "step-1",
				Status: cbpb.Build_SUCCESS,
				Timing: &cbpb.TimeSpan{
					StartTime: timestamppb.New(startTime),
					EndTime:   timestamppb.New(startTime.Add(30 * time.Second)),
				},
			},
			{
				Name:   "test-step-2",
				Id:     "step-2",
				Status: cbpb.Build_SUCCESS,
				Timing: &cbpb.TimeSpan{
					StartTime: timestamppb.New(startTime.Add(30 * time.Second)),
					EndTime:   timestamppb.New(startTime.Add(60 * time.Second)),
				},
			},
		},
	}
}

// createTagBuild creates a build with tag instead of branch
func createTagBuild(id, projectID string, status cbpb.Build_Status, startTime, finishTime time.Time) *cbpb.Build {
	build := createCompleteBuild(id, projectID, status, startTime, finishTime)
	build.Substitutions["BRANCH_NAME"] = ""
	build.Substitutions["TAG_NAME"] = "v1.0.0"
	return build
}

// createBuildWithoutRef creates a build without branch or tag
func createBuildWithoutRef(id, projectID string, status cbpb.Build_Status, startTime, finishTime time.Time) *cbpb.Build {
	build := createCompleteBuild(id, projectID, status, startTime, finishTime)
	build.Substitutions["BRANCH_NAME"] = ""
	build.Substitutions["TAG_NAME"] = ""
	return build
}

// createBuildWithoutTiming creates a build without start/finish times
func createBuildWithoutTiming(id, projectID string, status cbpb.Build_Status) *cbpb.Build {
	build := createCompleteBuild(id, projectID, status, time.Now(), time.Now().Add(time.Minute))
	build.StartTime = nil
	build.FinishTime = nil
	return build
}

// createBuildWithStepsWithoutTiming creates a build with steps that don't have timing
func createBuildWithStepsWithoutTiming(id, projectID string, status cbpb.Build_Status, startTime, finishTime time.Time) *cbpb.Build {
	build := createCompleteBuild(id, projectID, status, startTime, finishTime)
	build.Steps[0].Timing = nil
	build.Steps[1].Timing = nil
	return build
}

func TestSetUp(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *notifiers.Config
		wantErr bool
	}{
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
		// {
		// 	name: "valid url without auth",
		// 	cfg: &notifiers.Config{
		// 		Spec: &notifiers.Spec{
		// 			Notification: &notifiers.Notification{
		// 				Filter: "build.status == Build.Status.SUCCESS",
		// 				Delivery: map[string]interface{}{
		// 					"url": "http://example.com:9090/api/v1/write",
		// 				},
		// 			},
		// 		},
		// 	},
		// 	wantErr: false,
		// },
		{
			name: "with basic auth",
			cfg: &notifiers.Config{
				Spec: &notifiers.Spec{
					Notification: &notifiers.Notification{
						Filter: "build.status == Build.Status.SUCCESS",
						Delivery: map[string]interface{}{
							"url":      "http://example.com:9090/api/v1/write",
							"username": "michaelact",
							"password": map[interface{}]interface{}{
								"secretRef": "prometheus-password",
							},
						},
					},
					Secrets: []*notifiers.Secret{
						{
							LocalName:    "prometheus-password",
							ResourceName: "projects/test/secrets/prometheus-password/versions/latest",
						},
					},
				},
			},
			wantErr: false,
		},
		// {
		// 	name: "username without password",
		// 	cfg: &notifiers.Config{
		// 		Spec: &notifiers.Spec{
		// 			Notification: &notifiers.Notification{
		// 				Filter: "build.status == Build.Status.SUCCESS",
		// 				Delivery: map[string]interface{}{
		// 					"url":      "http://example.com:9090/api/v1/write",
		// 					"username": "michaelact",
		// 				},
		// 			},
		// 		},
		// 	},
		// 	wantErr: true,
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &prometheusNotifier{}
			err := p.SetUp(context.Background(), tt.cfg, "", new(fakeSecretGetter), nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetUp() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCollectMetrics(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		build          *cbpb.Build
		expectedCount  int
		description    string
	}{
		{
			name:          "complete build with all fields",
			build:         createCompleteBuild("test-build-1", "test-project", cbpb.Build_SUCCESS, now, now.Add(time.Minute)),
			expectedCount: 4, // build duration + 2 step durations + last run status
			description:   "Build with all timing information and 2 steps",
		},
		{
			name:          "tag build",
			build:         createTagBuild("test-build-2", "test-project", cbpb.Build_SUCCESS, now, now.Add(time.Minute)),
			expectedCount: 4, // build duration + 2 step durations + last run status
			description:   "Build with tag instead of branch",
		},
		{
			name:          "build without ref",
			build:         createBuildWithoutRef("test-build-3", "test-project", cbpb.Build_SUCCESS, now, now.Add(time.Minute)),
			expectedCount: 4, // build duration + 2 step durations + last run status
			description:   "Build without branch or tag information",
		},
		{
			name:          "build without timing",
			build:         createBuildWithoutTiming("test-build-4", "test-project", cbpb.Build_SUCCESS),
			expectedCount: 3, // 2 step durations + last run status (no build duration)
			description:   "Build without start/finish times",
		},
		{
			name:          "build with steps without timing",
			build:         createBuildWithStepsWithoutTiming("test-build-5", "test-project", cbpb.Build_SUCCESS, now, now.Add(time.Minute)),
			expectedCount: 2, // build duration + last run status (no step durations)
			description:   "Build with steps that don't have timing information",
		},
		{
			name:          "failed build",
			build:         createCompleteBuild("test-build-6", "test-project", cbpb.Build_FAILURE, now, now.Add(time.Minute)),
			expectedCount: 4, // build duration + 2 step durations + last run status
			description:   "Failed build with all timing information",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &prometheusNotifier{}
			metrics := p.collectMetrics(tt.build)

			// Verify metric count
			if len(metrics) != tt.expectedCount {
				t.Errorf("expected %d metrics, got %d (%s)", tt.expectedCount, len(metrics), tt.description)
			}

			// Verify all metrics have required fields
			for i, metric := range metrics {
				if len(metric.Labels) == 0 {
					t.Errorf("metric %d has no labels", i)
				}
				if len(metric.Samples) == 0 {
					t.Errorf("metric %d has no samples", i)
				}

				// Verify metric name is present
				hasName := false
				for _, label := range metric.Labels {
					if label.Name == "__name__" {
						hasName = true
						break
					}
				}
				if !hasName {
					t.Errorf("metric %d missing __name__ label", i)
				}
			}

			// Verify common labels are present in all metrics
			expectedLabels := []string{"cloud_account_id", "trigger_name", "repo_name", "commit_sha", "status", "machine_type", "ref_type", "ref"}
			for _, metric := range metrics {
				labelMap := make(map[string]string)
				for _, label := range metric.Labels {
					if label.Name != "__name__" {
						labelMap[label.Name] = label.Value
					}
				}

				for _, expectedLabel := range expectedLabels {
					if _, exists := labelMap[expectedLabel]; !exists {
						t.Errorf("metric missing expected label: %s", expectedLabel)
					}
				}
			}
		})
	}
}

func TestCollectMetricsSpecificValues(t *testing.T) {
	now := time.Now()
	build := createCompleteBuild("test-build", "test-project", cbpb.Build_SUCCESS, now, now.Add(time.Minute))

	p := &prometheusNotifier{}
	metrics := p.collectMetrics(build)

	// Verify build duration metric
	buildDuration := metrics[0]
	if buildDuration.Labels[0].Name != "__name__" || buildDuration.Labels[0].Value != "cloudbuild_build_duration_seconds" {
		t.Errorf("invalid metric name for build duration")
	}
	if buildDuration.Samples[0].Value != 60.0 { // 1 minute
		t.Errorf("expected build duration of 60 seconds, got %f", buildDuration.Samples[0].Value)
	}

	// Verify step duration metrics
	stepDuration1 := metrics[1]
	if stepDuration1.Labels[0].Name != "__name__" || stepDuration1.Labels[0].Value != "cloudbuild_step_duration_seconds" {
		t.Errorf("invalid metric name for step duration")
	}
	if stepDuration1.Samples[0].Value != 30.0 { // 30 seconds
		t.Errorf("expected first step duration of 30 seconds, got %f", stepDuration1.Samples[0].Value)
	}

	stepDuration2 := metrics[2]
	if stepDuration2.Samples[0].Value != 30.0 { // 30 seconds
		t.Errorf("expected second step duration of 30 seconds, got %f", stepDuration2.Samples[0].Value)
	}

	// Verify last run status metric
	lastRun := metrics[3]
	if lastRun.Labels[0].Name != "__name__" || lastRun.Labels[0].Value != "cloudbuild_build_last_run_status" {
		t.Errorf("invalid metric name for last run status")
	}
	if lastRun.Samples[0].Value != 1.0 {
		t.Errorf("expected last run status of 1.0, got %f", lastRun.Samples[0].Value)
	}

	// Verify specific label values
	labelMap := make(map[string]string)
	for _, label := range buildDuration.Labels {
		labelMap[label.Name] = label.Value
	}

	expectedValues := map[string]string{
		"cloud_account_id": "test-project",
		"trigger_name":     "test-trigger",
		"repo_name":        "test-repo",
		"commit_sha":       "abc123",
		"status":           "SUCCESS",
		"machine_type":     "E2_HIGHCPU_8",
		"ref_type":         "branch",
		"ref":              "main",
	}

	for key, expectedValue := range expectedValues {
		if actualValue, exists := labelMap[key]; !exists {
			t.Errorf("missing expected label: %s", key)
		} else if actualValue != expectedValue {
			t.Errorf("label %s: expected %s, got %s", key, expectedValue, actualValue)
		}
	}
}

func TestSendNotification(t *testing.T) {
	now := time.Now()
	build := createCompleteBuild("test-build", "test-project", cbpb.Build_SUCCESS, now, now.Add(time.Minute))

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
		{
			name:    "complex filter matches",
			filter:  "build.status == Build.Status.SUCCESS && build.substitutions['REPO_NAME'] == 'test-repo'",
			wantErr: false,
		},
		{
			name:    "complex filter does not match",
			filter:  "build.status == Build.Status.SUCCESS && build.substitutions['REPO_NAME'] == 'wrong-repo'",
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
							"url":      "http://example.com:9090/api/v1/write",
							"username": "michaelact",
							"password": map[interface{}]interface{}{
								"secretRef": "prometheus-password",
							},
						},
					},
					Secrets: []*notifiers.Secret{
						{
							LocalName:    "prometheus-password",
							ResourceName: "projects/test/secrets/prometheus-password/versions/latest",
						},
					},
				},
			}

			err := p.SetUp(context.Background(), cfg, "", new(fakeSecretGetter), nil)
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

func TestSendNotificationWithDifferentBuildTypes(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		build   *cbpb.Build
		filter  string
		wantErr bool
	}{
		{
			name:    "tag build with branch filter",
			build:   createTagBuild("tag-build", "test-project", cbpb.Build_SUCCESS, now, now.Add(time.Minute)),
			filter:  "build.substitutions['TAG_NAME'] != ''",
			wantErr: false,
		},
		{
			name:    "branch build with tag filter",
			build:   createCompleteBuild("branch-build", "test-project", cbpb.Build_SUCCESS, now, now.Add(time.Minute)),
			filter:  "build.substitutions['BRANCH_NAME'] != ''",
			wantErr: false,
		},
		{
			name:    "build without ref",
			build:   createBuildWithoutRef("no-ref-build", "test-project", cbpb.Build_SUCCESS, now, now.Add(time.Minute)),
			filter:  "build.status == Build.Status.SUCCESS",
			wantErr: false,
		},
		{
			name:    "failed build",
			build:   createCompleteBuild("failed-build", "test-project", cbpb.Build_FAILURE, now, now.Add(time.Minute)),
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
							"url": "http://example.com:9090/api/v1/write",
							"username": "michaelact",
							"password": map[interface{}]interface{}{
								"secretRef": "prometheus-password",
							},
						},
					},
					Secrets: []*notifiers.Secret{
						{
							LocalName:    "prometheus-password",
							ResourceName: "projects/test/secrets/prometheus-password/versions/latest",
						},
					},
				},
			}

			err := p.SetUp(context.Background(), cfg, "", new(fakeSecretGetter), nil)
			if err != nil {
				t.Fatalf("SetUp() error = %v", err)
			}

			err = p.SendNotification(context.Background(), tt.build)
			if (err != nil) != tt.wantErr {
				t.Errorf("SendNotification() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}