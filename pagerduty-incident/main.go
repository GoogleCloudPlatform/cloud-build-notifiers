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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	log "github.com/golang/glog"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
)

type pagerDutyIncidentBody struct {
	Incident pagerDutyIncident `json:"incident"`
}

type pagerDutyIncident struct {
	IncidentType string           `json:"type"`
	Title        string           `json:"title"`
	Service      pagerDutyService `json:"service"`
}

type pagerDutyService struct {
	ID          string `json:"id"`
	ServiceType string `json:"type"`
}

type pagerDutyIncidentNotifier struct {
	filter notifiers.EventFilter

	endpoint      string
	apiToken      string
	serviceID     string
	incidentTitle string
}

const (
	pagerDutyAPITokenSecretName = "pagerdutyAPIToken"
)

func main() {
	if err := notifiers.Main(new(pagerDutyIncidentNotifier)); err != nil {
		log.Fatalf("fatal error: %v", err)
	}
}

func (h *pagerDutyIncidentNotifier) SetUp(ctx context.Context, cfg *notifiers.Config, sg notifiers.SecretGetter, _ notifiers.BindingResolver) error {
	prd, err := notifiers.MakeCELPredicate(cfg.Spec.Notification.Filter)
	if err != nil {
		return fmt.Errorf("failed to create CELPredicate: %w", err)
	}
	h.filter = prd

	endpoint, ok := cfg.Spec.Notification.Delivery["incidentCreationEndpoint"].(string)
	if !ok {
		return fmt.Errorf("expected delivery config %v to have string field `endpoint`", cfg.Spec.Notification.Delivery)
	}
	h.endpoint = endpoint

	serviceID, ok := cfg.Spec.Notification.Delivery["serviceID"].(string)
	if !ok {
		return fmt.Errorf("expected delivery config %v to have string field `serviceID`", cfg.Spec.Notification.Delivery)
	}
	h.serviceID = serviceID

	incidentTitle, ok := cfg.Spec.Notification.Delivery["incidentTitle"].(string)
	if !ok {
		return fmt.Errorf("expected delivery config %v to have string field `incidentTitle`", cfg.Spec.Notification.Delivery)
	}
	h.incidentTitle = incidentTitle

	wuRef, err := notifiers.GetSecretRef(cfg.Spec.Notification.Delivery, pagerDutyAPITokenSecretName)
	if err != nil {
		return fmt.Errorf("failed to get Secret ref from delivery config (%v) field %q: %w", cfg.Spec.Notification.Delivery, pagerDutyAPITokenSecretName, err)
	}
	wuResource, err := notifiers.FindSecretResourceName(cfg.Spec.Secrets, wuRef)
	if err != nil {
		return fmt.Errorf("failed to find Secret for ref %q: %w", wuRef, err)
	}
	wu, err := sg.GetSecret(ctx, wuResource)
	if err != nil {
		return fmt.Errorf("failed to get token secret: %w", err)
	}
	h.apiToken = wu

	return nil
}

func (h *pagerDutyIncidentNotifier) SendNotification(ctx context.Context, build *cbpb.Build) error {
	if !h.filter.Apply(ctx, build) {
		log.V(2).Infof("not reporting an incident for event (build id = %s, status = %v)", build.Id, build.Status)
		return nil
	}

	log.Infof("reporting an incident for event (build id = %s, status = %s)", build.Id, build.Status)

	requestBody, err := json.Marshal(
		pagerDutyIncidentBody{
			Incident: pagerDutyIncident{
				IncidentType: "incident",
				Title:        h.incidentTitle,
				Service: pagerDutyService{
					ID:          h.serviceID,
					ServiceType: "service_reference",
				},
			},
		},
	)

	if err != nil {
		fmt.Println(err)
	}

	log.Infof("incident request body: %v", string(requestBody))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("failed to create a new HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "GCB-Notifier/0.1 (http)")
	req.Header.Set("Authorization", fmt.Sprintf("Token token=%s", h.apiToken))
	req.Header.Set("Accept", "application/vnd.pagerduty+json;version=2")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		log.Warningf("got a non-OK response status %q (%d) from %q", resp.Status, resp.StatusCode, h.endpoint)
	}

	log.V(2).Infoln("send HTTP request successfully")
	return nil
}
