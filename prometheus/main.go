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
	"net/url"
	"time"

	cbpb "cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	log "github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	promconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"
)

const (
	urlKey      = "url"
	usernameKey = "username"
	passwordKey = "password"
)

func main() {
	log.Infof("Starting Prometheus notifier...")
	if err := notifiers.Main(new(prometheusNotifier)); err != nil {
		log.Fatalf("fatal error: %v", err)
	}
}

type prometheusNotifier struct {
	filter       notifiers.EventFilter
	client       remote.WriteClient
	clientConfig *remote.ClientConfig
}

func (p *prometheusNotifier) SetUp(ctx context.Context, cfg *notifiers.Config, _ string, sg notifiers.SecretGetter, _ notifiers.BindingResolver) error {
	log.Infof("Setting up Prometheus notifier with config: %+v", cfg.Spec.Notification.Delivery)

	// Set up CEL filter
	log.V(2).Infof("Creating CEL predicate from filter: %v", cfg.Spec.Notification.Filter)
	prd, err := notifiers.MakeCELPredicate(cfg.Spec.Notification.Filter)
	if err != nil {
		log.Errorf("Failed to make CEL predicate: %v", err)
		return fmt.Errorf("failed to make a CEL predicate: %w", err)
	}
	p.filter = prd
	log.V(2).Infof("CEL predicate created successfully")

	// Validate and get remote write URL
	urlStr, ok := cfg.Spec.Notification.Delivery[urlKey].(string)
	if !ok || urlStr == "" {
		log.Errorf("Missing or invalid delivery.url in config")
		return fmt.Errorf("delivery.url is required")
	}
	log.V(2).Infof("Using Prometheus remote write URL: %s", urlStr)

	// Parse URL to validate format
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		log.Errorf("Failed to parse URL %s: %v", urlStr, err)
		return fmt.Errorf("invalid delivery.url: %w", err)
	}
	log.V(2).Infof("URL parsed successfully: %s", parsedURL.String())

	// Configure basic auth if username is provided
	var httpConfig promconfig.HTTPClientConfig
	if username, ok := cfg.Spec.Notification.Delivery[usernameKey].(string); ok && username != "" {
		log.V(2).Infof("Configuring basic auth for username: %s", username)

		// Get password from secret if username is set
		passwordRef, err := notifiers.GetSecretRef(cfg.Spec.Notification.Delivery, passwordKey)
		if err != nil {
			log.Errorf("Failed to get password secret reference: %v", err)
			return fmt.Errorf("password secret reference is required when username is set: %w", err)
		}
		log.V(2).Infof("Password secret reference: %s", passwordRef)

		passwordResource, err := notifiers.FindSecretResourceName(cfg.Spec.Secrets, passwordRef)
		if err != nil {
			log.Errorf("Failed to find secret resource for ref %q: %v", passwordRef, err)
			return fmt.Errorf("failed to find Secret for ref %q: %w", passwordRef, err)
		}
		log.V(2).Infof("Password secret resource: %s", passwordResource)

		password, err := sg.GetSecret(ctx, passwordResource)
		if err != nil {
			log.Errorf("Failed to get password secret: %v", err)
			return fmt.Errorf("failed to get password secret: %w", err)
		}
		log.V(3).Infof("Password secret retrieved successfully (length: %d)", len(password))

		httpConfig = promconfig.HTTPClientConfig{
			BasicAuth: &promconfig.BasicAuth{
				Username: username,
				Password: promconfig.Secret(password),
			},
		}
		log.V(2).Infof("Basic auth configured successfully")
	} else {
		log.V(2).Infof("No basic auth configured - using unauthenticated requests")
	}

	p.clientConfig = &remote.ClientConfig{
		URL: &promconfig.URL{
			URL: parsedURL,
		},
		Timeout:          model.Duration(30 * time.Second),
		HTTPClientConfig: httpConfig,
		Headers: map[string]string{
			"X-Prometheus-Remote-Write-Version": "0.1.0",
		},
		RetryOnRateLimit: true,
	}
	log.V(2).Infof("Client config created: timeout=%v, retryOnRateLimit=%v", p.clientConfig.Timeout, p.clientConfig.RetryOnRateLimit)

	client, err := remote.NewWriteClient("cloudbuild", p.clientConfig)
	if err != nil {
		log.Errorf("Failed to create remote write client: %v", err)
		return fmt.Errorf("failed to create remote write client: %w", err)
	}
	p.client = client
	log.Infof("Prometheus notifier setup completed successfully")
	return nil
}

func (p *prometheusNotifier) SendNotification(ctx context.Context, build *cbpb.Build) error {
	log.V(2).Infof("Processing build notification: ID=%s, Status=%s, Project=%s", build.Id, build.Status, build.ProjectId)

	if !p.filter.Apply(ctx, build) {
		log.V(2).Infof("Build filtered out (build id = %s, status = %v)", build.Id, build.Status)
		return nil
	}

	log.Infof("Sending metrics for Build %q (status: %q, project: %q)", build.Id, build.Status, build.ProjectId)

	// Collect metrics from build
	log.V(2).Infof("Collecting metrics for build %s", build.Id)
	metrics := p.collectMetrics(build)
	log.V(2).Infof("Collected %d metrics for build %s", len(metrics), build.Id)

	// Write metrics to Prometheus
	if err := p.writeMetrics(ctx, metrics); err != nil {
		log.Errorf("Failed to write metrics for build %s: %v", build.Id, err)
		return fmt.Errorf("failed to write metrics: %w", err)
	}

	log.Infof("Successfully sent %d metrics for build %s", len(metrics), build.Id)
	return nil
}

// CAUTION: HIGH CARDINALITY
// Ref: https://prometheus.io/docs/practices/naming/#:~:text=CAUTION%3A%20Remember,sets%20of%20values.
func (p *prometheusNotifier) collectMetrics(build *cbpb.Build) []prompb.TimeSeries {
	var metrics []prompb.TimeSeries

	// Get common labels
	commonLabels := map[string]string{
		"cloud_account_id": build.ProjectId,
		"trigger_name":     build.Substitutions["TRIGGER_NAME"],
		"repo_name":        build.Substitutions["REPO_NAME"],
		// "commit_sha":       build.Substitutions["SHORT_SHA"], // CAUTION: HIGH CARDINALITY
		"status":           build.Status.String(),
		"machine_type":     build.Options.GetMachineType().String(),
	}
	log.V(3).Infof("Common labels for build %s: %+v", build.Id, commonLabels)

	// Add branch/tag information
	if branch := build.Substitutions["BRANCH_NAME"]; branch != "" {
		commonLabels["ref_type"] = "branch"
		commonLabels["ref"] = branch
		log.V(3).Infof("Build %s is on branch: %s", build.Id, branch)
	} else if tag := build.Substitutions["TAG_NAME"]; tag != "" {
		commonLabels["ref_type"] = "tag"
		commonLabels["ref"] = tag
		log.V(3).Infof("Build %s is on tag: %s", build.Id, tag)
	} else {
		commonLabels["ref_type"] = "unknown"
		commonLabels["ref"] = "[no branch or tag]"
		log.V(3).Infof("Build %s has no branch or tag information", build.Id)
	}

	// Add failure information if available
	if build.FailureInfo != nil {
		commonLabels["failure_type"] = build.FailureInfo.GetType().String()
		// commonLabels["failure_detail"] = build.FailureInfo.GetDetail() // CAUTION: HIGH CARDINALITY
	}

	// Build duration metric
	if build.StartTime != nil && build.FinishTime != nil {
		duration := build.FinishTime.AsTime().Sub(build.StartTime.AsTime()).Seconds()
		timestamp := build.FinishTime.AsTime().UnixNano() / int64(time.Millisecond)
		log.V(3).Infof("Build %s duration: %.2f seconds", build.Id, duration)
		metrics = append(metrics, p.createHistogramMetric(
			"cloudbuild_build_duration_seconds",
			duration,
			commonLabels,
			timestamp,
		))
	} else {
		log.V(2).Infof("Build %s missing start/finish time - skipping duration metric", build.Id)
	}

	// Step duration metrics
	log.V(3).Infof("Processing %d steps for build %s", len(build.Steps), build.Id)
	for i, step := range build.Steps {
		if step.Timing != nil {
			duration := step.Timing.EndTime.AsTime().Sub(step.Timing.StartTime.AsTime()).Seconds()
			timestamp := step.Timing.EndTime.AsTime().UnixNano() / int64(time.Millisecond)
			stepLabels := make(map[string]string)
			for k, v := range commonLabels {
				stepLabels[k] = v
			}
			stepLabels["step_name"] = step.Name
			stepLabels["step_status"] = step.Status.String()
			stepLabels["step_id"] = step.Id

			log.V(3).Infof("Step %d (%s) duration: %.2f seconds, status: %s", i+1, step.Name, duration, step.Status)
			metrics = append(metrics, p.createHistogramMetric(
				"cloudbuild_step_duration_seconds",
				duration,
				stepLabels,
				timestamp,
			))
		} else {
			log.V(3).Infof("Step %d (%s) missing timing information - skipping duration metric", i+1, step.Name)
		}
	}

	// Last run status metric
	var statusTimestamp int64
	if build.FinishTime != nil {
		statusTimestamp = build.FinishTime.AsTime().UnixNano() / int64(time.Millisecond)
	} else if build.StartTime != nil {
		statusTimestamp = build.StartTime.AsTime().UnixNano() / int64(time.Millisecond)
	} else {
		statusTimestamp = time.Now().UnixNano() / int64(time.Millisecond)
	}

	var lastRunStatusValue float64
	if build.Status == cbpb.Build_SUCCESS {
		lastRunStatusValue = 1.0
	} else {
		lastRunStatusValue = 0.0
	}

	metrics = append(metrics, p.createGaugeMetric(
		"cloudbuild_build_last_run_status",
		lastRunStatusValue,
		commonLabels,
		statusTimestamp,
	))
	log.V(3).Infof("Added last run status metric for build %s", build.Id)

	return metrics
}

func (p *prometheusNotifier) writeMetrics(ctx context.Context, metrics []prompb.TimeSeries) error {
	log.V(2).Infof("Preparing to write %d metrics to Prometheus", len(metrics))

	req := &prompb.WriteRequest{
		Timeseries: metrics,
	}

	data, err := proto.Marshal(req)
	if err != nil {
		log.Errorf("Failed to marshal write request: %v", err)
		return fmt.Errorf("failed to marshal write request: %w", err)
	}
	log.V(3).Infof("Marshaled write request: %d bytes", len(data))

	compressed := snappy.Encode(nil, data)
	log.V(3).Infof("Compressed data: %d bytes (compression ratio: %.2f)", len(compressed), float64(len(compressed))/float64(len(data)))

	log.V(2).Infof("Sending metrics to Prometheus remote write endpoint")
	err = p.client.Store(ctx, compressed, 0)
	if err != nil {
		log.Errorf("Failed to store metrics: %v", err)
		return err
	}

	log.V(2).Infof("Successfully stored %d metrics to Prometheus", len(metrics))
	return nil
}

func (p *prometheusNotifier) createHistogramMetric(name string, value float64, labels map[string]string, timestamp int64) prompb.TimeSeries {
	log.V(4).Infof("Creating histogram metric: %s = %f with labels: %+v, timestamp: %d", name, value, labels, timestamp)
	return prompb.TimeSeries{
		Labels: p.createLabels(name, labels),
		Samples: []prompb.Sample{
			{
				Value:     value,
				Timestamp: timestamp,
			},
		},
	}
}

func (p *prometheusNotifier) createGaugeMetric(name string, value float64, labels map[string]string, timestamp int64) prompb.TimeSeries {
	log.V(4).Infof("Creating gauge metric: %s = %f with labels: %+v, timestamp: %d", name, value, labels, timestamp)
	return prompb.TimeSeries{
		Labels: p.createLabels(name, labels),
		Samples: []prompb.Sample{
			{
				Value:     value,
				Timestamp: timestamp,
			},
		},
	}
}

func (p *prometheusNotifier) createLabels(name string, labels map[string]string) []prompb.Label {
	result := []prompb.Label{
		{
			Name:  "__name__",
			Value: name,
		},
	}
	for k, v := range labels {
		result = append(result, prompb.Label{
			Name:  k,
			Value: v,
		})
	}
	log.V(4).Infof("Created %d labels for metric %s", len(result), name)
	return result
}
