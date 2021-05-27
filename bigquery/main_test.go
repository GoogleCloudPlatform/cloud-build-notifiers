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
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"

	"cloud.google.com/go/bigquery"
	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	log "github.com/golang/glog"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"google.golang.org/api/googleapi"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeLayer struct {
	size int64
}

func (ml *fakeLayer) Digest() (v1.Hash, error) {
	return v1.Hash{}, nil
}

func (ml *fakeLayer) DiffID() (v1.Hash, error) {
	return v1.Hash{}, nil
}

func (ml *fakeLayer) Compressed() (io.ReadCloser, error) {
	return nil, nil
}
func (ml *fakeLayer) Uncompressed() (io.ReadCloser, error) {
	return nil, nil
}
func (ml *fakeLayer) Size() (int64, error) {
	return ml.size, nil

}
func (ml *fakeLayer) MediaType() (types.MediaType, error) {
	return "", nil
}

type fakeBQ struct {
	validSchema bool
	writtenRows []*bqRow
}

type fakeBQFactory struct {
	fake *fakeBQ
}

func (bqf *fakeBQFactory) Make(ctx context.Context) (bq, error) {
	return bqf.fake, nil
}

const tableURI = "projects/project_name/datasets/dataset_name/tables/valid"

var fakeBQServerDS = map[string]fakeDMResponse{
	"dne":     {&bigquery.DatasetMetadata{}, &googleapi.Error{Code: http.StatusNotFound, Message: "not found"}},
	"noauth":  {&bigquery.DatasetMetadata{}, &googleapi.Error{Code: http.StatusForbidden, Message: "no authorization"}},
	"broke":   {&bigquery.DatasetMetadata{}, &googleapi.Error{Code: http.StatusInternalServerError, Message: "bq server error"}},
	"strange": {&bigquery.DatasetMetadata{}, &googleapi.Error{Code: http.StatusProxyAuthRequired, Message: "use proxy"}},
}

var fakeBQServerTable = map[string]fakeTMResponse{
	"dne":            {&bigquery.TableMetadata{}, &googleapi.Error{Code: http.StatusNotFound, Message: "not found"}},
	"noauth":         {&bigquery.TableMetadata{}, &googleapi.Error{Code: http.StatusForbidden, Message: "no authorization"}},
	"broke":          {&bigquery.TableMetadata{}, &googleapi.Error{Code: http.StatusInternalServerError, Message: "bq server error"}},
	"strange":        {&bigquery.TableMetadata{}, &googleapi.Error{Code: http.StatusProxyAuthRequired, Message: "use proxy"}},
	"notinitialized": {&bigquery.TableMetadata{Name: "name"}, nil},
	"empty":          {&bigquery.TableMetadata{Schema: bigquery.Schema{}}, nil},
	"notempty":       {&bigquery.TableMetadata{Schema: bigquery.Schema{&bigquery.FieldSchema{Name: "field"}}}, nil},
}

type fakeDMResponse struct {
	dataset   *bigquery.DatasetMetadata
	fakeError error
}

type fakeTMResponse struct {
	table     *bigquery.TableMetadata
	fakeError error
}

func (bq *fakeBQ) EnsureDataset(ctx context.Context, datasetName string) error {
	fakeResponse := fakeBQServerDS[datasetName]
	err := fakeResponse.fakeError
	if err != nil {
		log.Warningf("Error obtaining dataset metadata: %v", err)
		if strings.Contains(err.Error(), "404") {
			return nil
		}
		if strings.Contains(err.Error(), "403") {
			return fmt.Errorf("Error creating dataset: %v", err.Error())
		}
		if strings.Contains(err.Error(), "500") {
			return fmt.Errorf("Error creating dataset: %v", err.Error())
		}
		return fmt.Errorf("Encountered error %v: ", err.Error())
	}
	return nil
}

func (bq *fakeBQ) EnsureTable(ctx context.Context, tableName string) error {
	if tableName == "valid" {
		bq.validSchema = true
		return nil
	}
	bq.validSchema = false
	fakeResponse := fakeBQServerTable[tableName]
	err := fakeResponse.fakeError
	if err != nil {
		log.Warningf("Error obtaining table metadata: %v", err)
		if strings.Contains(err.Error(), "404") {
			return nil
		}
		if strings.Contains(err.Error(), "403") {
			return fmt.Errorf("Failed to initialize table %v: ", err.Error())
		}
		if strings.Contains(err.Error(), "500") {
			return fmt.Errorf("Failed to initialize table %v: ", err.Error())
		}
		return fmt.Errorf("Encountered error %v: ", err.Error())
	}
	if len(fakeResponse.table.Schema) == 0 {
		bq.validSchema = true
	}
	return nil
}

func (bq *fakeBQ) WriteRow(ctx context.Context, row *bqRow) error {
	if !bq.validSchema {
		return errors.New("Error writing to table, invalid schema")
	}
	if row == nil {
		return errors.New("cannot insert empty row")
	}
	bq.writtenRows = append(bq.writtenRows, row)
	return nil
}

func TestSetUp(t *testing.T) {

	for _, tc := range []struct {
		name    string
		cfg     *notifiers.Config
		wantErr bool
	}{{
		name: "valid config",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.build_trigger_id == "123e4567-e89b-12d3-a456-426614174000" `,
					Delivery: map[string]interface{}{
						"table": tableURI,
					},
				},
			},
		},
	}, {
		name: "missing filter",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Delivery: map[string]interface{}{
						"table": tableURI,
					},
				},
			},
		},
		wantErr: true,
	}, {
		name: "bad filter",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: "uh oh",
					Delivery: map[string]interface{}{
						"table": tableURI,
					},
				},
			},
		},
		wantErr: true,
	}, {
		name: "missing table URI",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.build_trigger_id == "123e4567-e89b-12d3-a456-426614174000" `,
					Delivery: map[string]interface{}{
						"uh": "oh",
					},
				},
			},
		},
		wantErr: true,
	}, {
		name: "invalid `table URI`",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.build_trigger_id == "123e4567-e89b-12d3-a456-426614174000" `,
					Delivery: map[string]interface{}{
						"table": "my_big_query_table",
					},
				},
			},
		},
		wantErr: true,
	}, {
		name: "non-string `table`",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.build_trigger_id == "123e4567-e89b-12d3-a456-426614174000" `,
					Delivery: map[string]interface{}{
						"table": 404,
					},
				},
			},
		},
		wantErr: true,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			n := &bqNotifier{bqf: &fakeBQFactory{&fakeBQ{}}}
			err := n.SetUp(context.Background(), tc.cfg, nil, nil)
			if err != nil {
				if tc.wantErr {
					t.Logf("got expected error: %v", err)
					return
				}
				t.Fatalf("SetUp(%v) got unexpected error: %v", tc.cfg, err)
			}

			if tc.wantErr {
				t.Error("unexpected success")
			}

		})
	}
}

func TestEnsureFunctions(t *testing.T) {
	const tableURI = "projects/project_name/datasets/dataset_name/tables/valid"

	for _, tc := range []struct {
		name    string
		cfg     *notifiers.Config
		wantErr bool
	}{{
		name: "valid dataset and table, functional BQ",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.build_trigger_id == "123e4567-e89b-12d3-a456-426614174000" `,
					Delivery: map[string]interface{}{
						"table": tableURI,
					},
				},
			},
		},
	}, {
		name: "create nonexistent dataset",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.build_trigger_id == "123e4567-e89b-12d3-a456-426614174000" `,
					Delivery: map[string]interface{}{
						"table": "projects/project_name/datasets/dne/tables/valid",
					},
				},
			},
		},
	}, {
		name: "create nonexistent table",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.build_trigger_id == "123e4567-e89b-12d3-a456-426614174000" `,
					Delivery: map[string]interface{}{
						"table": "projects/project_name/datasets/dataset_name/tables/dne",
					},
				},
			},
		},
	}, {
		name: "no perm dataset",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.build_trigger_id == "123e4567-e89b-12d3-a456-426614174000" `,
					Delivery: map[string]interface{}{
						"table": "projects/project_name/datasets/noauth/tables/valid",
					},
				},
			},
		},
		wantErr: true,
	}, {
		name: "no perm table",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.build_trigger_id == "123e4567-e89b-12d3-a456-426614174000" `,
					Delivery: map[string]interface{}{
						"table": "projects/project_name/datasets/dataset_name/tables/noauth",
					},
				},
			},
		},
		wantErr: true,
	}, {
		name: "server error",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.build_trigger_id == "123e4567-e89b-12d3-a456-426614174000" `,
					Delivery: map[string]interface{}{
						"table": "projects/project_name/datasets/dataset_name/tables/broke",
					},
				},
			},
		},
		wantErr: true,
	}, {
		name: "some non 200 error",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.build_trigger_id == "123e4567-e89b-12d3-a456-426614174000" `,
					Delivery: map[string]interface{}{
						"table": "projects/project_name/datasets/dataset_name/tables/strange",
					},
				},
			},
		},
		wantErr: true,
	},
		{
			name: "more server error",
			cfg: &notifiers.Config{
				Spec: &notifiers.Spec{
					Notification: &notifiers.Notification{
						Filter: `build.build_trigger_id == "123e4567-e89b-12d3-a456-426614174000" `,
						Delivery: map[string]interface{}{
							"table": "projects/project_name/datasets/broke/tables/valid",
						},
					},
				},
			},
			wantErr: true,
		}} {
		t.Run(tc.name, func(t *testing.T) {
			n := &bqNotifier{bqf: &fakeBQFactory{&fakeBQ{}}}
			err := n.SetUp(context.Background(), tc.cfg, nil, nil)
			if err != nil {
				if tc.wantErr {
					t.Logf("got expected error: %v", err)
					return
				}
				t.Fatalf("SetUp(%v) got unexpected error: %v", tc.cfg, err)
			}

			if tc.wantErr {
				t.Error("unexpected success")
			}

		})
	}
}

func TestSendNotification(t *testing.T) {
	const tableURI = "projects/project_name/datasets/dataset_name/tables/valid"

	for _, tc := range []struct {
		name    string
		cfg     *notifiers.Config
		build   *cbpb.Build
		wantErr bool
		wantRow bool
	}{{
		name: "missing IDs",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.build_trigger_id == "1234" `,
					Delivery: map[string]interface{}{
						"table": tableURI,
					},
				},
			},
		},
		build: &cbpb.Build{
			BuildTriggerId: "1234",
			Id:             "1",
			Status:         cbpb.Build_SUCCESS,
		},
		wantErr: true,
		wantRow: false,
	}, {
		name: "unknown status",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.build_trigger_id == "1234" `,
					Delivery: map[string]interface{}{
						"table": tableURI,
					},
				},
			},
		},
		build: &cbpb.Build{
			ProjectId:      "Project ID",
			Id:             "Build ID",
			BuildTriggerId: "1234",
			Status:         cbpb.Build_STATUS_UNKNOWN,
		},
		wantErr: false,
		wantRow: false,
	}, {
		name: "successful build",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.build_trigger_id == "1234" `,
					Delivery: map[string]interface{}{
						"table": tableURI,
					},
				},
			},
		},
		build: &cbpb.Build{
			ProjectId:      "Project ID",
			Id:             "Build ID",
			BuildTriggerId: "1234",
			Status:         cbpb.Build_SUCCESS,
			CreateTime:     timestamppb.Now(),
			StartTime:      timestamppb.Now(),
			FinishTime:     timestamppb.Now(),
			Tags:           []string{},
			Options:        &cbpb.BuildOptions{Env: []string{}},
		},
		wantErr: false,
		wantRow: true,
	}, {
		name: "no timestamp build",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.build_trigger_id == "1234" `,
					Delivery: map[string]interface{}{
						"table": tableURI,
					},
				},
			},
		},
		build: &cbpb.Build{
			ProjectId:      "Project ID",
			Id:             "Build ID",
			BuildTriggerId: "1234",
			Status:         cbpb.Build_FAILURE,
		},
		wantErr: true,
		wantRow: false,
	}, {
		name: "table is empty",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.build_trigger_id == "123e4567-e89b-12d3-a456-426614174000" `,
					Delivery: map[string]interface{}{
						"table": "projects/project_name/datasets/dataset_name/tables/empty",
					},
				},
			},
		},
		build: &cbpb.Build{
			ProjectId:      "Project ID",
			Id:             "Build ID",
			BuildTriggerId: "123e4567-e89b-12d3-a456-426614174000",
			Status:         cbpb.Build_SUCCESS,
			CreateTime:     timestamppb.Now(),
			StartTime:      timestamppb.Now(),
			FinishTime:     timestamppb.Now(),
			Tags:           []string{},
			Options:        &cbpb.BuildOptions{Env: []string{}},
		},
		wantErr: false,
		wantRow: true,
	}, {
		name: "schema not initialized",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.build_trigger_id == "123e4567-e89b-12d3-a456-426614174000" `,
					Delivery: map[string]interface{}{
						"table": "projects/project_name/datasets/dataset_name/tables/notinitialized",
					},
				},
			},
		},
		build: &cbpb.Build{
			ProjectId:      "Project ID",
			Id:             "Build ID",
			BuildTriggerId: "123e4567-e89b-12d3-a456-426614174000",
			Status:         cbpb.Build_SUCCESS,
			CreateTime:     timestamppb.Now(),
			StartTime:      timestamppb.Now(),
			FinishTime:     timestamppb.Now(),
			Tags:           []string{},
			Options:        &cbpb.BuildOptions{Env: []string{}},
		},
		wantErr: false,
		wantRow: true,
	}, {
		name: "no build step timing",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.build_trigger_id == "123e4567-e89b-12d3-a456-426614174000" `,
					Delivery: map[string]interface{}{
						"table": "projects/project_name/datasets/dataset_name/tables/notinitialized",
					},
				},
			},
		},
		build: &cbpb.Build{
			ProjectId:      "Project ID",
			Id:             "Build ID",
			BuildTriggerId: "123e4567-e89b-12d3-a456-426614174000",
			Status:         cbpb.Build_SUCCESS,
			CreateTime:     timestamppb.Now(),
			StartTime:      timestamppb.Now(),
			FinishTime:     timestamppb.Now(),
			Tags:           []string{},
			Steps:          []*cbpb.BuildStep{{Name: "test"}},
			Options:        &cbpb.BuildOptions{Env: []string{}},
		},
		wantErr: false,
		wantRow: true,
	}, {
		name: "table exists with bad schema",
		cfg: &notifiers.Config{
			Spec: &notifiers.Spec{
				Notification: &notifiers.Notification{
					Filter: `build.build_trigger_id == "123e4567-e89b-12d3-a456-426614174000" `,
					Delivery: map[string]interface{}{
						"table": "projects/project_name/datasets/dataset_name/tables/notempty",
					},
				},
			},
		},
		build: &cbpb.Build{
			ProjectId:      "Project ID",
			Id:             "Build ID",
			BuildTriggerId: "123e4567-e89b-12d3-a456-426614174000",
			Status:         cbpb.Build_SUCCESS,
			CreateTime:     timestamppb.Now(),
			StartTime:      timestamppb.Now(),
			FinishTime:     timestamppb.Now(),
			Tags:           []string{},
			Options:        &cbpb.BuildOptions{Env: []string{}},
		},
		wantErr: true,
		wantRow: false,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			fakeBQ := &fakeBQ{}
			n := &bqNotifier{bqf: &fakeBQFactory{fakeBQ}}
			err := n.SetUp(context.Background(), tc.cfg, nil, nil)
			if err != nil {
				t.Fatalf("Setup(%v) got unexpected error: %v", tc.cfg, err)
			}
			err = n.SendNotification(context.Background(), tc.build)
			if err != nil {
				if tc.wantErr {
					t.Logf("got expected error: %v", err)
					return
				}
				t.Fatalf("Send Notification(%v) got unexpected error: %v", tc.build, err)
			}
			if tc.wantErr {
				t.Error("unexpected success")
			}
			if !tc.wantRow && len(fakeBQ.writtenRows) != 0 {
				t.Errorf("unexpected write: %v", fakeBQ.writtenRows)
			}
			if tc.wantRow && len(fakeBQ.writtenRows) == 0 {
				t.Error("expected write")
			}
		})
	}
}

func TestGetImageSize(t *testing.T) {
	for _, tc := range []struct {
		name      string
		layers    []v1.Layer
		totalSize *big.Rat
		wantErr   bool
	}{{
		name: "valid layers",
		layers: []v1.Layer{
			&fakeLayer{
				size: 10,
			},
			&fakeLayer{
				size: 20,
			},
		},
		totalSize: big.NewRat(30, megaByte),
		wantErr:   false,
	}, {
		name:      "no layers",
		layers:    []v1.Layer{},
		totalSize: big.NewRat(0, megaByte),
		wantErr:   false,
	}, {
		name: "empty layers",
		layers: []v1.Layer{
			&fakeLayer{},
			&fakeLayer{
				size: 20,
			},
		},
		totalSize: big.NewRat(20, int64(1000000)),
		wantErr:   false,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			calculatedSize, err := getImageSize(tc.layers)
			if err != nil {
				if tc.wantErr {
					t.Logf("got expected error: %v", err)
					return
				}
				t.Fatalf("getImageSize got unexpected error: %v", err)
			}
			if calculatedSize.Cmp(tc.totalSize) != 0 {
				t.Errorf("Expected %v, received %v", tc.totalSize, calculatedSize)
			}
			if tc.wantErr {
				t.Error("unexpected success")
			}
		})
	}
}

func TestInferSchema(t *testing.T) {
	if _, err := bigquery.InferSchema(bqRow{}); err != nil {
		t.Errorf("Failed to infer schema: %v", err)
	}
}
