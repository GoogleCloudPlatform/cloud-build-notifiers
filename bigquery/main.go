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
	"math/big"
	"os"
	"regexp"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	log "github.com/golang/glog"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
)

var rgp = regexp.MustCompile(".*/.*/.*/(.*)/.*/(.*)")

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
	ProjectID       string
	ID              string
	BuildTriggerID  string
	Status          string
	ContainerSizeMB *big.Rat
	Steps           []buildStep
	CreateTime      civil.Time
	StartTime       civil.Time
	FinishTime      civil.Time
	Tags            []string
	Env             []string
}

type buildStep struct {
	Name      string
	ID        string
	Status    string
	Args      []string
	StartTime civil.Time
	EndTime   civil.Time
}

func (bq *bqNotifier) SetUp(ctx context.Context, cfg *notifiers.Config, _ notifiers.SecretGetter) error {
	projectID := os.Getenv("PROJECT_ID")
	if projectID == "" {
		return errors.New("PROJECT_ID environment variable must be set")
	}

	prd, err := notifiers.MakeCELPredicate(cfg.Spec.Notification.Filter)
	if err != nil {
		return fmt.Errorf("failed to make a CEL predicate: %v", err)
	}
	parsed, ok := cfg.Spec.Notification.Delivery["table"].(string)
	if !ok {
		return fmt.Errorf("Expected table string: %v", cfg.Spec.Notification.Delivery)
	}

	// Initialize client
	bq.filter = prd
	bq.client, err = bigquery.NewClient(ctx, projectID)
	if err != nil {
		return fmt.Errorf("Failed to initialize bigquery client: %v", err)
	}

	// Extract dataset id and table id from config
	rs := rgp.FindStringSubmatch(parsed)
	bq.dataset = bq.client.Dataset(rs[1])
	bq.table = bq.dataset.Table(rs[2])

	// Check for existence of dataset, create if false
	_, err = bq.dataset.Metadata(ctx)
	if err != nil {
		log.Warningf("Error obtaining dataset metadata: %v", err)
		if err := bq.dataset.Create(ctx, &bigquery.DatasetMetadata{Name: rs[1], Description: "BigQuery Notifier Build Data"}); err != nil {
			return fmt.Errorf("Error creating dataset: %v", err)
		}
	}

	// Check for existence of table, create if false
	_, err = bq.table.Metadata(ctx)
	if err != nil {
		log.Warningf("Error obtaining table metadata: %v", err)
		schema, err := bigquery.InferSchema(bqRow{})
		if err != nil {
			return fmt.Errorf("Failed to infer schema: %v", err)
		}
		// Create table if it does not exist.
		if err := bq.table.Create(ctx, &bigquery.TableMetadata{Name: rs[2], Description: "BigQuery Notifier Build Data Table", Schema: schema}); err != nil {
			return fmt.Errorf("Failed to initialize table %v: ", err)
		}
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
