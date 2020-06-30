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
	"errors"
	"fmt"
	"os"

	"cloud.google.com/go/bigquery"
	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	log "github.com/golang/glog"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
)

func main() {
	if err := notifiers.Main(new(bqNotifier)); err != nil {
		log.Fatalf("fatal error: %v", err)
	}
}

type bqNotifier struct {
	filter  notifiers.EventFilter
	client  *bigquery.Client
	dataset *bigquery.Dataset
	table   *bigquery.Table
}

type bqRow struct {
	ProjectID      string
	ID             string
	BuildTriggerID string
	Status         string
}

func (bq *bqNotifier) SetUp(ctx context.Context, cfg *notifiers.Config, _ notifiers.SecretGetter) error {
	projectID := os.Getenv("PROJECT_ID")
	if projectID == "" {
		return errors.New("PROJECT_ID environment variable must be set")
	}

	prd, err := notifiers.MakeCELPredicate(cfg.Spec.Notification.Filter)
	if err != nil {
		return fmt.Errorf("failed to make a CEL predicate: %w", err)
	}
	_, ok := cfg.Spec.Notification.Delivery["table"].(string)
	if !ok {
		return fmt.Errorf("Expected table string: %v", cfg.Spec.Notification.Delivery)
	}

	bq.filter = prd
	bq.client, err = bigquery.NewClient(ctx, projectID)
	if err != nil {
		return fmt.Errorf("Failed to initialize bigquery client: %w", err)
	}

	return nil

}

func (bq *bqNotifier) SendNotification(ctx context.Context, build *cbpb.Build) error {
	if !bq.filter.Apply(ctx, build) {
		log.V(2).Infof("Not doing BQ write for build %v", build.Id)
		return nil
	}
	if build.BuildTriggerId == "" {
		log.Warningf("Build passes filter but does not have trigger ID: %v, status: %v", bq.filter, build.GetStatus())
	}
	log.Infof("sending Big Query write for build %q (status: %q)", build.Id, build.Status)
	return nil
}
