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
	"fmt"
	"net/http"
    "strings"

	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	log "github.com/golang/glog"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
)

func main() {
	if err := notifiers.Main(new(teamsNotifier)); err != nil {
		log.Fatalf("fatal error: %v", err)
	}
}

type teamsNotifier struct {
	filter notifiers.EventFilter
	url    string
}

func (h *teamsNotifier) SetUp(_ context.Context, cfg *notifiers.Config, _ notifiers.SecretGetter) error {
	prd, err := notifiers.MakeCELPredicate(cfg.Spec.Notification.Filter)
	if err != nil {
		return fmt.Errorf("failed to create CELPredicate: %w", err)
	}
	h.filter = prd

	url, ok := cfg.Spec.Notification.Delivery["url"].(string)
	if !ok {
		return fmt.Errorf("expected delivery config %v to have string field `url`", cfg.Spec.Notification.Delivery)
	}
	h.url = url

	return nil
}

func (h *teamsNotifier) SendNotification(ctx context.Context, build *cbpb.Build) error {
	if !h.filter.Apply(ctx, build) {
		log.V(2).Infof("not sending HTTP request for event (build id = %s, status = %v)", build.Id, build.Status)
		return nil
	}

	log.Infof("sending HTTP request for event (build id = %s, status = %s)", build.Id, build.Status)

	var clr string
	switch build.Status {
	case cbpb.Build_SUCCESS:
		clr = "7CFC00"
	case cbpb.Build_FAILURE, cbpb.Build_INTERNAL_ERROR, cbpb.Build_TIMEOUT:
		clr = "FF0000"
	default:
		clr = "FF8C00"
	}

	jsonTemplate := []byte(`
	{
		"@context": "https://schema.org/extensions",
		"@type": "MessageCard",
		"themeColor": "%s",
		"text": "Cloud Build (%s, %s): %s",
		"potentialAction": [
			{
				"@type": "OpenUri",
				"name": "View Logs",
				"isPrimary": true,
				"targets": [
					{ "os": "default", "uri": "%s" }
				]
			}
		]
	}`)

	jsonTxt := fmt.Sprintf(
		string(jsonTemplate),
		clr,
		build.ProjectId,
		build.Id,
		build.Status,
		build.LogUrl,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, strings.NewReader(jsonTxt))
	if err != nil {
		return fmt.Errorf("failed to create a new HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "GCB-Notifier/0.1 (teams)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Warningf("got a non-OK response status %q (%d) from %q", resp.Status, resp.StatusCode, h.url)
	}

	log.V(2).Infoln("send HTTP request successfully")
	return nil
}