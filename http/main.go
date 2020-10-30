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
	"fmt"
	"net/http"

	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	log "github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

func main() {
	if err := notifiers.Main(new(httpNotifier)); err != nil {
		log.Fatalf("fatal error: %v", err)
	}
}

type httpNotifier struct {
	filter notifiers.EventFilter
	url    string
}

func (h *httpNotifier) SetUp(_ context.Context, cfg *notifiers.Config, _ notifiers.SecretGetter, _ notifiers.BindingResolver) error {
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

func (h *httpNotifier) SendNotification(ctx context.Context, build *cbpb.Build) error {
	if !h.filter.Apply(ctx, build) {
		log.V(2).Infof("not sending HTTP request for event (build id = %s, status = %v)", build.Id, build.Status)
		return nil
	}

	log.Infof("sending HTTP request for event (build id = %s, status = %s)", build.Id, build.Status)

	logURL, err := notifiers.AddUTMParams(build.LogUrl, notifiers.HTTPMedium)
	if err != nil {
		return fmt.Errorf("failed to add UTM params: %w", err)
	}
	build.LogUrl = logURL

	mo := protojson.MarshalOptions{}
	jb, err := mo.Marshal(proto.MessageV2(build))
	if err != nil {
		return fmt.Errorf("failed to marshal Build proto to JSON: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, bytes.NewReader(jb))
	if err != nil {
		return fmt.Errorf("failed to create a new HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "GCB-Notifier/0.1 (http)")

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
