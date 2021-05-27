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
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	log "github.com/golang/glog"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/google"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var tableResource = regexp.MustCompile(".*/.*/.*/(.*)/.*/(.*)")

var terminalStatusCodes = map[cbpb.Build_Status]bool{
	cbpb.Build_SUCCESS:        true,
	cbpb.Build_FAILURE:        true,
	cbpb.Build_INTERNAL_ERROR: true,
	cbpb.Build_TIMEOUT:        true,
	cbpb.Build_CANCELLED:      true,
	cbpb.Build_EXPIRED:        true,
}

// TODO(aricz)
const megaByte = int64(1000000)

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
	CreateTime     civil.DateTime
	StartTime      civil.DateTime
	FinishTime     civil.DateTime
	Tags           []string
	Env            []string
	LogURL         string
	Substitutions  []*substitution
}

type substitution struct {
	Key   string
	Value string
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
	StartTime civil.DateTime
	EndTime   civil.DateTime
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
		return nil, fmt.Errorf("error initializing bigquery client: %v", err)
	}
	newClient := &actualBQ{client: bqClient}
	return newClient, nil
}

func getImageSize(layers []v1.Layer) (*big.Rat, error) {
	totalSum := int64(0)
	for _, layer := range layers {
		layerSize, err := layer.Size()
		if err != nil {
			return nil, fmt.Errorf("error parsing layer %v: %v", layer, err)
		}
		totalSum += layerSize
	}
	return big.NewRat(totalSum, megaByte), nil
}

func imageManifestToBuildImage(image string) (*buildImage, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return nil, fmt.Errorf("error parsing image reference: %v", err)
	}
	img, err := remote.Image(ref, remote.WithAuthFromKeychain(google.Keychain))
	if err != nil {
		return nil, fmt.Errorf("error obtaining image reference: %v", err)
	}
	sha, err := img.Digest()
	layers, err := img.Layers()
	// Calculating the compressed image size
	containerSize, err := getImageSize(layers)
	if err != nil {
		return nil, err
	}

	return &buildImage{SHA: sha.String(), ContainerSizeMB: containerSize}, nil
}

func (n *bqNotifier) SetUp(ctx context.Context, cfg *notifiers.Config, _ notifiers.SecretGetter, _ notifiers.BindingResolver) error {
	prd, err := notifiers.MakeCELPredicate(cfg.Spec.Notification.Filter)
	if err != nil {
		return fmt.Errorf("failed to make a CEL predicate: %v", err)
	}
	parsed, ok := cfg.Spec.Notification.Delivery["table"].(string)
	if !ok {
		return fmt.Errorf("expected table string: %v", cfg.Spec.Notification.Delivery)
	}

	// Initialize client
	n.filter = prd
	n.client, err = n.bqf.Make(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize bigquery client: %v", err)
	}

	// Extract dataset id and table id from config
	rs := tableResource.FindStringSubmatch(parsed)
	if len(rs) != 3 {
		return fmt.Errorf("failed to parse valid table URI: %v", parsed)
	}
	if err = n.client.EnsureDataset(ctx, rs[1]); err != nil {
		return err
	}
	if err = n.client.EnsureTable(ctx, rs[2]); err != nil {
		return err
	}
	return nil

}

func parsePBTime(time *timestamppb.Timestamp) (civil.DateTime, error) {
	newTime, err := ptypes.Timestamp(time)
	if err != nil {
		return civil.DateTime{}, fmt.Errorf("error parsing timestamp: %v", err)
	}
	return civil.DateTimeOf(newTime), nil
}

func (n *bqNotifier) SendNotification(ctx context.Context, build *cbpb.Build) error {
	if !n.filter.Apply(ctx, build) {
		log.V(2).Infof("not doing BQ write for build %v", build.Id)
		return nil
	}
	if build.BuildTriggerId == "" {
		log.Warningf("build passes filter but does not have a trigger ID. Build id: %q, status: %v", build.Id, build.GetStatus())
	}
	if !terminalStatusCodes[build.Status] {
		log.Infof("not writing to BigQuery for non-terminal build status %v", build.Status.String())
		return nil
	}
	log.Infof("sending Big Query write for build %q (status: %q)", build.Id, build.Status)
	if build.ProjectId == "" {
		return fmt.Errorf("build missing project id")
	}
	buildImages := []*buildImage{}
	shaSet := make(map[string]bool)
	for _, image := range build.GetImages() {
		buildImage, err := imageManifestToBuildImage(image)
		if err != nil {
			return fmt.Errorf("error parsing image manifest: %v", err)
		}
		if shaSet[buildImage.SHA] {
			continue
		}
		shaSet[buildImage.SHA] = true
		buildImages = append(buildImages, buildImage)
	}
	buildSteps := []*buildStep{}
	createTime, err := parsePBTime(build.CreateTime)
	if err != nil {
		return fmt.Errorf("error parsing CreateTime: %v", err)
	}
	startTime, err := parsePBTime(build.StartTime)
	if err != nil {
		return fmt.Errorf("error parsing StartTime: %v", err)
	}
	finishTime, err := parsePBTime(build.FinishTime)
	if err != nil {
		return fmt.Errorf("error parsing FinishTime: %v", err)
	}
	unixZeroTimestamp, err := ptypes.TimestampProto(time.Unix(0, 0))
	if err != nil {
		return err
	}
	for _, step := range build.GetSteps() {
		st := step.GetTiming().GetStartTime()
		et := step.GetTiming().GetEndTime()
		if st == nil {
			st = unixZeroTimestamp
		}
		if et == nil {
			et = unixZeroTimestamp
		}
		startTime, err := parsePBTime(st)
		if err != nil {
			return fmt.Errorf("error parsing StartTime: %v", err)
		}
		endTime, err := parsePBTime(et)
		if err != nil {
			return fmt.Errorf("error parsing EndTime: %v", err)
		}
		newStep := &buildStep{
			Name:      step.Name,
			ID:        step.Id,
			Status:    step.GetStatus().String(),
			Args:      step.Args,
			StartTime: startTime,
			EndTime:   endTime,
		}
		buildSteps = append(buildSteps, newStep)
	}
	logURL, err := notifiers.AddUTMParams(build.LogUrl, notifiers.StorageMedium)
	if err != nil {
		return fmt.Errorf("Error generating UTM params: %v", err)
	}
	substitutions := []*substitution{}
	for key, value := range build.Substitutions {
		substitutions = append(substitutions, &substitution{key, value})
	}
	newRow := &bqRow{
		ProjectID:      build.ProjectId,
		ID:             build.Id,
		BuildTriggerID: build.BuildTriggerId,
		Status:         build.Status.String(),
		Images:         buildImages,
		Steps:          buildSteps,
		CreateTime:     createTime,
		StartTime:      startTime,
		FinishTime:     finishTime,
		Tags:           build.Tags,
		Env:            build.Options.Env,
		LogURL:         logURL,
		Substitutions:  substitutions,
	}
	return n.client.WriteRow(ctx, newRow)
}
func (bq *actualBQ) EnsureDataset(ctx context.Context, datasetName string) error {
	// Check for existence of dataset, create if false
	bq.dataset = bq.client.Dataset(datasetName)
	_, err := bq.client.Dataset(datasetName).Metadata(ctx)
	if err != nil {
		log.Warningf("error obtaining dataset metadata: %v;Creating new BigQuery dataset: %q", err, datasetName)
		if err := bq.dataset.Create(ctx, &bigquery.DatasetMetadata{
			Name: datasetName, Description: "BigQuery Notifier Build Data",
		}); err != nil {
			return fmt.Errorf("error creating dataset: %v", err)
		}
	}
	return nil
}

func (bq *actualBQ) EnsureTable(ctx context.Context, tableName string) error {
	// Check for existence of table, create if false
	bq.table = bq.dataset.Table(tableName)
	schema, err := bigquery.InferSchema(bqRow{})
	if err != nil {
		return fmt.Errorf("Failed to infer schema: %v", err)
	}
	metadata, err := bq.dataset.Table(tableName).Metadata(ctx)
	if err != nil {
		log.Warningf("Error obtaining table metadata: %q;Creating new BigQuery table: %q", err, tableName)
		// Create table if it does not exist.
		if err := bq.table.Create(ctx, &bigquery.TableMetadata{Name: tableName, Description: "BigQuery Notifier Build Data Table", Schema: schema}); err != nil {
			return fmt.Errorf("Failed to initialize table %v: ", err)
		}
	} else if len(metadata.Schema) == 0 {
		log.Warningf("No schema found for table, writing new schema for table: %v", tableName)
		update := bigquery.TableMetadataToUpdate{
			Schema: schema,
		}
		if _, err := bq.table.Update(ctx, update, metadata.ETag); err != nil {
			return fmt.Errorf("Error: unable to update schema of table: %v", err)
		}
	}

	return nil
}

func (bq *actualBQ) WriteRow(ctx context.Context, row *bqRow) error {
	ins := bq.table.Inserter()
	log.V(2).Infof("Writing row: %v", row)
	if err := ins.Put(ctx, row); err != nil {
		return fmt.Errorf("Error inserting row into BQ: %v", err)
	}
	return nil
}
