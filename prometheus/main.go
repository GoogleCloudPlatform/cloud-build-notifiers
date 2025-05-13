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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"
	log "github.com/golang/glog"
)

const (
	urlKey = "url"
	usernameKey = "username"
	passwordKey = "password"
)

func main() {
	if err := notifiers.Main(new(prometheusNotifier)); err != nil {
		log.Fatalf("fatal error: %v", err)
	}
}

type prometheusNotifier struct {
	filter notifiers.EventFilter
	client *remote.Client
	config *remote.ClientConfig
}

func (p *prometheusNotifier) SetUp(ctx context.Context, cfg *notifiers.Config, _ string, sg notifiers.SecretGetter, _ notifiers.BindingResolver) error {
	// Set up CEL filter
	prd, err := notifiers.MakeCELPredicate(cfg.Spec.Notification.Filter)
	if err != nil {
		return fmt.Errorf("failed to make a CEL predicate: %w", err)
	}
	p.filter = prd

	// Validate and get remote write URL
	urlStr, ok := cfg.Spec.Notification.Delivery[urlKey].(string)
	if !ok || urlStr == "" {
		return fmt.Errorf("delivery.url is required")
	}

	// Parse URL to validate format
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid delivery.url: %w", err)
	}

	// Configure basic auth if username is provided
	var httpConfig config.HTTPClientConfig
	if username, ok := cfg.Spec.Notification.Delivery[usernameKey].(string); ok && username != "" {
		// Get password from secret if username is set
		passwordRef, err := notifiers.GetSecretRef(cfg.Spec.Notification.Delivery, passwordKey)
		if err != nil {
			return fmt.Errorf("password secret reference is required when username is set: %w", err)
		}
		passwordResource, err := notifiers.FindSecretResourceName(cfg.Spec.Secrets, passwordRef)
		if err != nil {
			return fmt.Errorf("failed to find Secret for ref %q: %w", passwordRef, err)
		}
		password, err := sg.GetSecret(ctx, passwordResource)
		if err != nil {
			return fmt.Errorf("failed to get password secret: %w", err)
		}

		httpConfig = config.HTTPClientConfig{
			BasicAuth: &config.BasicAuth{
				Username: username,
				Password: config.Secret(password),
			},
		}
	}

	p.config = &remote.ClientConfig{
		URL: &config_util.URL{
			URL: parsedURL,
		},
		Timeout: model.Duration(30 * time.Second),
		HTTPClientConfig: httpConfig,
		Headers: map[string]string{
			"X-Prometheus-Remote-Write-Version": "0.1.0",
		},
		RetryOnRateLimit: true,
		WriteProtoMsg:    config.RemoteWriteProtoMsgV1,
	}

	client, err := remote.NewWriteClient("cloudbuild", p.config)
	if err != nil {
		return fmt.Errorf("failed to create remote write client: %w", err)
	}
	p.client = client

	return nil
}

func (p *prometheusNotifier) SendNotification(ctx context.Context, build *cbpb.Build) error {
	if !p.filter.Apply(ctx, build) {
		log.V(2).Infof("not sending metrics for event (build id = %s, status = %v)", build.Id, build.Status)
		return nil
	}

	log.Infof("sending metrics for Build %q (status: %q)", build.Id, build.Status)

	// Collect metrics from build
	metrics := p.collectMetrics(build)

	// Write metrics to Prometheus
	if err := p.writeMetrics(ctx, metrics); err != nil {
		return fmt.Errorf("failed to write metrics: %w", err)
	}

	log.V(2).Infoln("metrics sent successfully")
	return nil
}

func (p *prometheusNotifier) collectMetrics(build *cbpb.Build) []prompb.TimeSeries {
	var metrics []prompb.TimeSeries

	// Get common labels
	commonLabels := map[string]string{
		"cloud_account_id": build.ProjectId,
		"trigger_name":     build.Substitutions["TRIGGER_NAME"],
		"repo_name":        build.Substitutions["REPO_NAME"],
		"commit_sha":       build.Substitutions["SHORT_SHA"],
		"status":          build.Status.String(),
		"machine_type":    build.Options.GetMachineType().String(),
	}

	// Add branch/tag information
	if branch := build.Substitutions["BRANCH_NAME"]; branch != "" {
		commonLabels["ref_type"] = "branch"
		commonLabels["ref"] = branch
	} else if tag := build.Substitutions["TAG_NAME"]; tag != "" {
		commonLabels["ref_type"] = "tag"
		commonLabels["ref"] = tag
	} else {
		commonLabels["ref_type"] = "unknown"
		commonLabels["ref"] = "[no branch or tag]"
	}

	// Add failure information if available
	// CAUTION: HIGH CARDINALITY
	// Ref: https://prometheus.io/docs/practices/naming/#:~:text=CAUTION%3A%20Remember,sets%20of%20values.
	// if build.FailureInfo != nil {
	// 	commonLabels["failure_type"] = build.FailureInfo.GetType().String()
	// 	commonLabels["failure_detail"] = build.FailureInfo.GetDetail()
	// }

	// Build duration metric
	if build.StartTime != nil && build.FinishTime != nil {
		duration := build.FinishTime.AsTime().Sub(build.StartTime.AsTime()).Seconds()
		metrics = append(metrics, p.createHistogramMetric(
			"cloudbuild_build_duration_seconds",
			duration,
			commonLabels,
		))
	}

	// Step duration metrics
	for _, step := range build.Steps {
		if step.Timing != nil {
			duration := step.Timing.EndTime.AsTime().Sub(step.Timing.StartTime.AsTime()).Seconds()
			stepLabels := make(map[string]string)
			for k, v := range commonLabels {
				stepLabels[k] = v
			}
			stepLabels["step_name"] = step.Name
			stepLabels["step_status"] = step.Status.String()
			stepLabels["step_id"] = step.Id

			metrics = append(metrics, p.createHistogramMetric(
				"cloudbuild_step_duration_seconds",
				duration,
				stepLabels,
			))
		}
	}

	// Last run status metric
	metrics = append(metrics, p.createGaugeMetric(
		"cloudbuild_build_last_run_status",
		1.0,
		commonLabels,
	))

	return metrics
}

func (p *prometheusNotifier) writeMetrics(ctx context.Context, metrics []prompb.TimeSeries) error {
	req := &prompb.WriteRequest{
		Timeseries: metrics,
	}

	data, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal write request: %w", err)
	}

	compressed := snappy.Encode(nil, data)
	_, err = p.client.Store(ctx, compressed, 0)
	return err
}

func (p *prometheusNotifier) createHistogramMetric(name string, value float64, labels map[string]string) prompb.TimeSeries {
	return prompb.TimeSeries{
		Labels: p.createLabels(name, labels),
		Samples: []prompb.Sample{
			{
				Value:     value,
				Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
			},
		},
	}
}

func (p *prometheusNotifier) createGaugeMetric(name string, value float64, labels map[string]string) prompb.TimeSeries {
	return prompb.TimeSeries{
		Labels: p.createLabels(name, labels),
		Samples: []prompb.Sample{
			{
				Value:     value,
				Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
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
	return result
}