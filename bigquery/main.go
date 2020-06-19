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
	"os"
	"strings"

	log "github.com/golang/glog"

	"cloud.google.com/go/bigquery"
	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
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

func (bq *bqNotifier) SetUp(ctx context.Context, cfg *notifiers.Config, sg notifiers.SecretGetter) error {
	projectID := os.Getenv("PROJECT_ID")
	if projectID == "" {
		fmt.Println("PROJECT_ID environment variable must be set.")
		os.Exit(1)
	}

	prd, err := notifiers.MakeCELPredicate(cfg.Spec.Notification.Filter)
	if err != nil {
		return fmt.Errorf("failed to make a CEL predicate: %v", err)
	}
	tableString, ok := cfg.Spec.Notification.Delivery["table"].(string)
	if !ok {
		return fmt.Errorf("Expected table string")
	}

	parsed := strings.Split(tableString, "/")
	dataset := parsed[len(parsed)-3]
	table := parsed[len(parsed)-1]

	bq.filter = prd
	bq.client, err = bigquery.NewClient(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to initialize bigquery client")
	}
	bq.dataset = bq.client.Dataset(dataset)
	bq.table = bq.dataset.Table(table)
	// table existence check
	_, err = bq.table.Metadata(ctx)
	if err != nil {
		// create table
		schema, err := bigquery.InferSchema(bqRow{})
		if err != nil {
			// failed to infer schema
			fmt.Println("Failed to infer schema")
		}
		tableMD := bigquery.TableMetadata{Name: table, Schema: schema}
		if err := bq.table.Create(ctx, &tableMD); err != nil {
			return fmt.Errorf("Failed to initialize table %v: %v", table, err)
		}
	}
	return nil

}

func (bq *bqNotifier) SendNotification(ctx context.Context, build *cbpb.Build) error {
	if !bq.filter.Apply(ctx, build) {
		return nil
	}
	log.Infof("sending Big Query write for build %q (status: %q)", build.Id, build.Status)
	fmt.Println(build)
	return nil
}
