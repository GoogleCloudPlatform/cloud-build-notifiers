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
	"strings"
	"testing"

	"cloud.google.com/go/bigquery"
	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	log "github.com/golang/glog"
	"google.golang.org/api/googleapi"
)

type mockBQ struct {
}

type mockBQFactory struct {
}

func (bqf *mockBQFactory) Make(ctx context.Context) (bq, error) {
	return &mockBQ{}, nil
}

var fakeBQServerDS = map[string]fakeDMResponse{
	"dne":     {&bigquery.DatasetMetadata{}, &googleapi.Error{Code: 404, Message: "not found"}},
	"noauth":  {&bigquery.DatasetMetadata{}, &googleapi.Error{Code: 403, Message: "no authorization"}},
	"broke":   {&bigquery.DatasetMetadata{}, &googleapi.Error{Code: 500, Message: "bq server error"}},
	"strange": {&bigquery.DatasetMetadata{}, &googleapi.Error{Code: 305, Message: "use proxy"}},
}

var fakeBQServerTable = map[string]fakeTMResponse{
	"dne":     {&bigquery.TableMetadata{}, &googleapi.Error{Code: 404, Message: "not found"}},
	"noauth":  {&bigquery.TableMetadata{}, &googleapi.Error{Code: 403, Message: "no authorization"}},
	"broke":   {&bigquery.TableMetadata{}, &googleapi.Error{Code: 500, Message: "bq server error"}},
	"strange": {&bigquery.TableMetadata{}, &googleapi.Error{Code: 305, Message: "use proxy"}},
}

type fakeDMResponse struct {
	dataset   *bigquery.DatasetMetadata
	fakeError error
}

type fakeTMResponse struct {
	table     *bigquery.TableMetadata
	fakeError error
}

func (bq *mockBQ) EnsureDataset(ctx context.Context, datasetName string) error {
	fakeResponse, _ := fakeBQServerDS[datasetName]
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

func (bq *mockBQ) EnsureTable(ctx context.Context, tableName string) error {
	fakeResponse, _ := fakeBQServerTable[tableName]
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
	return nil
}

func (bq *mockBQ) WriteRow(ctx context.Context, row *bqRow) error {
	return nil
}

func TestSetUp(t *testing.T) {
	const tableURI = "projects/project_name/datasets/dataset_name/tables/table_name"

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
			n := &bqNotifier{bqf: &mockBQFactory{}}
			err := n.SetUp(context.Background(), tc.cfg, nil)
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
						"table": "projects/project_name/datasets/dataset_name/tables/table_name",
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
						"table": "projects/project_name/datasets/dne/tables/table_name",
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
						"table": "projects/project_name/datasets/noauth/tables/table_name",
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
							"table": "projects/project_name/datasets/broke/tables/table_name",
						},
					},
				},
			},
			wantErr: true,
		}} {
		t.Run(tc.name, func(t *testing.T) {
			n := &bqNotifier{bqf: &mockBQFactory{}}
			err := n.SetUp(context.Background(), tc.cfg, nil)
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
