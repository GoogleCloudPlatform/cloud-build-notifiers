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
	"github.com/golang/protobuf/ptypes"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/google"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
)

var tableResource = regexp.MustCompile(".*/.*/.*/(.*)/.*/(.*)")
var terminalStatusCodes = []string{"SUCCESS", "FAILURE"}

func main() {
	if err := notifiers.Main(&bqNotifier{bqf: &actualBQFactory{}}); err != nil {
		log.Fatalf("fatal error: %v", err)
	}
}

type bqNotifier struct {
	bqf    bqFactory
	filter notifiers.EventFilter
	client bq
}

type bqRow struct {
	ProjectID      string
	ID             string
	BuildTriggerID string
	Status         string
	Images         []*buildImage
	Steps          []*buildStep
	CreateTime     civil.Time
	StartTime      civil.Time
	FinishTime     civil.Time
	Tags           []string
	Env            []string
}

type buildImage struct {
	SHA             string
	ContainerSizeMB *big.Rat
}

type buildStep struct {
	Name      string
	ID        string
	Status    string
	Args      []string
	StartTime civil.Time
	EndTime   civil.Time
}

type actualBQ struct {
	client  *bigquery.Client
	dataset *bigquery.Dataset
	table   *bigquery.Table
}

type actualBQFactory struct {
}

func (bqf *actualBQFactory) Make(ctx context.Context) (bq, error) {

	projectID := os.Getenv("PROJECT_ID")
	if projectID == "" {
		return nil, errors.New("PROJECT_ID environment variable must be set")
	}
	bqClient, err := bigquery.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("Error intializing bigquery client: %v", err)
	}
	newClient := &actualBQ{client: bqClient}
	return newClient, nil
}

func parseImageManifest(image string) (*buildImage, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return nil, fmt.Errorf("Error parsing image reference: %v", err)
	}
	img, err := remote.Image(ref, remote.WithAuthFromKeychain(google.Keychain))
	if err != nil {
		return nil, fmt.Errorf("Error obtaining image reference: %v", err)
	}
	sha, err := img.Digest()
	layers, err := img.Layers()
	totalSum := int64(0)
	for _, layer := range layers {
		layerSize, err := layer.Size()
		if err != nil {
			return nil, fmt.Errorf("Error parsing layer %v: %v", layer, err)
		}
		totalSum += layerSize
	}
	containerSize := big.NewRat(totalSum, int64(1000000))
	return &buildImage{SHA: sha.String(), ContainerSizeMB: containerSize}, nil
}

func (n *bqNotifier) SetUp(ctx context.Context, cfg *notifiers.Config, _ notifiers.SecretGetter) error {
	prd, err := notifiers.MakeCELPredicate(cfg.Spec.Notification.Filter)
	if err != nil {
		return fmt.Errorf("failed to make a CEL predicate: %v", err)
	}
	parsed, ok := cfg.Spec.Notification.Delivery["table"].(string)
	if !ok {
		return fmt.Errorf("Expected table string: %v", cfg.Spec.Notification.Delivery)
	}

	// Initialize client
	n.filter = prd
	n.client, err = n.bqf.Make(ctx)
	if err != nil {
		return err
	}

	if err != nil {
		return fmt.Errorf("Failed to initialize bigquery client: %v", err)
	}

	// Extract dataset id and table id from config
	rs := tableResource.FindStringSubmatch(parsed)
	if len(rs) != 3 {
		return fmt.Errorf("Failed to parse valid table URI: %v", parsed)
	}
	if err = n.client.EnsureDataset(ctx, rs[1]); err != nil {
		return err
	}
	if err = n.client.EnsureTable(ctx, rs[2]); err != nil {
		return err
	}
	return nil

}

func (n *bqNotifier) SendNotification(ctx context.Context, build *cbpb.Build) error {
	if !n.filter.Apply(ctx, build) {
		log.V(2).Infof("Not doing BQ write for build %v", build.Id)
		return nil
	}
	if build.BuildTriggerId == "" {
		log.Warningf("Build passes filter but does not have trigger ID: %v, status: %v", n.filter, build.GetStatus())
	}
	log.Infof("sending Big Query write for build %q (status: %q)", build.Id, build.Status)
	if build.ProjectId == "" {
		return fmt.Errorf("Build missing project id")
	}
	if build.Id == "" {
		return fmt.Errorf("Build missing id")
	}
	newRow := bqRow{}
	newRow.ProjectID = build.ProjectId
	newRow.ID = build.Id
	newRow.BuildTriggerID = build.BuildTriggerId
	newRow.Status = build.Status.String()
	if newRow.Status == "STATUS_UNKNOWN" {
		return fmt.Errorf("Build %v has unknown status", newRow.ID)
	}
	terminalBuild := false
	for _, status := range terminalStatusCodes {
		if status == newRow.Status {
			terminalBuild = true
		}
	}
	if !terminalBuild {
		log.Infof("Not writing non-terminal build step %v", newRow.Status)
		return nil
	}

	log.Infof("BUILD IMAGES %v", build.GetImages())
	newRow.Images = []*buildImage{}
	for _, image := range build.GetImages() {
		buildImage, err := parseImageManifest(image)
		if err != nil {
			return fmt.Errorf("Error parsing image manifest: %v", err)
		}
		newRow.Images = append(newRow.Images, buildImage)
	}

	newRow.Steps = []*buildStep{}
	createTimePre, err := ptypes.Timestamp(build.CreateTime)
	if err != nil {
		return fmt.Errorf("Error parsing createTime: %v", err)
	}
	createTime := civil.TimeOf(createTimePre)
	startTimePre, err := ptypes.Timestamp(build.CreateTime)
	if err != nil {
		return fmt.Errorf("Error parsing createTime: %v", err)
	}
	startTime := civil.TimeOf(startTimePre)
	finishTimePre, err := ptypes.Timestamp(build.CreateTime)
	if err != nil {
		return fmt.Errorf("Error parsing createTime: %v", err)
	}
	finishTime := civil.TimeOf(finishTimePre)
	newRow.CreateTime = createTime
	newRow.StartTime = startTime
	newRow.FinishTime = finishTime
	newRow.Tags = build.Tags
	newRow.Env = build.Options.Env

	for _, step := range build.GetSteps() {
		newStep := &buildStep{}
		newStep.Name = step.Name
		newStep.ID = step.Id
		newStep.Status = step.GetStatus().String()
		newStep.Args = step.Args
		startTimePre, err := ptypes.Timestamp(step.Timing.StartTime)
		if err != nil {
			return fmt.Errorf("Error parsing start time for step: %v", err)
		}
		startTime := civil.TimeOf(startTimePre)
		endTimePre, err := ptypes.Timestamp(step.Timing.EndTime)
		if err != nil {
			return fmt.Errorf("Error parsing end time for step: %v", err)
		}
		endTime := civil.TimeOf(endTimePre)
		newStep.StartTime = startTime
		newStep.EndTime = endTime
		newRow.Steps = append(newRow.Steps, newStep)
	}
	err = n.client.WriteRow(ctx, &newRow)
	return nil
}
func (bq *actualBQ) EnsureDataset(ctx context.Context, datasetName string) error {
	// Check for existence of dataset, create if false
	bq.dataset = bq.client.Dataset(datasetName)
	_, err := bq.client.Dataset(datasetName).Metadata(ctx)
	if err != nil {
		log.Warningf("Error obtaining dataset metadata: %v", err)
		if err := bq.dataset.Create(ctx, &bigquery.DatasetMetadata{
			Name: datasetName, Description: "BigQuery Notifier Build Data",
		}); err != nil {
			return fmt.Errorf("Error creating dataset: %v", err)
		}
	}
	return nil
}

func (bq *actualBQ) EnsureTable(ctx context.Context, tableName string) error {
	// Check for existence of table, create if false
	bq.table = bq.dataset.Table(tableName)
	_, err := bq.dataset.Table(tableName).Metadata(ctx)
	if err != nil {
		log.Warningf("Error obtaining table metadata: %v", err)
		schema, err := bigquery.InferSchema(bqRow{})
		if err != nil {
			return fmt.Errorf("Failed to infer schema: %v", err)
		}
		// Create table if it does not exist.
		if err := bq.table.Create(ctx, &bigquery.TableMetadata{Name: tableName, Description: "BigQuery Notifier Build Data Table", Schema: schema}); err != nil {
			return fmt.Errorf("Failed to initialize table %v: ", err)
		}
	}
	return nil
}

func (bq *actualBQ) WriteRow(ctx context.Context, row *bqRow) error {
	ins := bq.table.Inserter()
	log.Infof("Writing row: %v", row)
	if err := ins.Put(ctx, row); err != nil {
		return fmt.Errorf("Error inserting row into BQ: %v", err)
	}
	return nil
}
