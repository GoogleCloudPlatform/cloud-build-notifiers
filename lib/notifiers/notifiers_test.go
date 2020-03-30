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

package notifiers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestMakeCELPredicate(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name      string
		filter    string
		event     *CloudBuildEvent
		wantMatch bool
	}{
		{
			name:      "id match",
			filter:    `event.id == "abc"`,
			event:     &CloudBuildEvent{ID: "abc"},
			wantMatch: true,
		}, {
			name:      "id mismatch",
			filter:    `event.id == "abc"`,
			event:     &CloudBuildEvent{ID: "def"},
			wantMatch: false,
		}, {
			name:      "complex filter match",
			filter:    `event.buildTriggerId == "trigger-id" && event.status == "SUCCESS" && "blah" in event.tags`,
			event:     &CloudBuildEvent{BuildTriggerID: "trigger-id", Status: "SUCCESS", Tags: []string{"blah"}},
			wantMatch: true,
		}, {
			name:      "complex filter mismatch",
			filter:    `event.buildTriggerId == "trigger-id" && event.status == "SUCCESS" && size(event.tags) == 2 && "bar" in event.tags`,
			event:     &CloudBuildEvent{BuildTriggerID: "trigger-id", Status: "SUCCESS", Tags: []string{"foo", "baz"}},
			wantMatch: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pred, err := MakeCELPredicate(tc.filter)
			if err != nil {
				t.Fatalf("MakeCELProgram(%q): %v", tc.filter, err)
			}

			if pred.Apply(ctx, tc.event) != tc.wantMatch {
				t.Errorf("CELPredicate(%+v) != %v", tc.event, tc.wantMatch)
			}
		})
	}
}

type fakeGCSReaderFactory struct {
	// A mapping of "gs://"+bucket+"/"+object -> content.
	data map[string]string
}

func (f *fakeGCSReaderFactory) NewReader(_ context.Context, bucket, object string) (io.ReadCloser, error) {
	s, ok := f.data["gs://"+bucket+"/"+object]
	if !ok {
		return nil, fmt.Errorf("no data for bucket=%q object=%q", bucket, object)
	}

	return ioutil.NopCloser(bytes.NewBufferString(s)), nil
}

const validConfigYAML = `
apiVersion: gcb-notifiers/v1alpha1
kind: TestNotifier
metadata:
  name: my-test-notifier
spec:
  notification:
    filter: event.status == "SUCCESS"
    delivery:
      some_key: some_value
      other_key: [404, 505]
      third_key:
        foo: bar
  secrets:
    - name: some-secret
      value: projects/my-project/secrets/my-secret/versions/latest
`

var validConfig = &Config{
	APIVersion: "gcb-notifiers/v1alpha1",
	Kind:       "TestNotifier",
	Metadata: &Metadata{
		Name: "my-test-notifier",
	},
	Spec: &Spec{
		Notification: &Notification{
			Filter: `event.status == "SUCCESS"`,
			Delivery: map[string]interface{}{
				"some_key":  "some_value",
				"other_key": []interface{}{int(404), int(505)},
				"third_key": map[interface{}]interface{}{string("foo"): string("bar")},
			},
		},
		Secrets: []*Secret{
			&Secret{
				LocalName:    "some-secret",
				ResourceName: "projects/my-project/secrets/my-secret/versions/latest",
			},
		},
	},
}

var validFakeFactory = &fakeGCSReaderFactory{
	data: map[string]string{
		"gs://path/to/my/config.yaml": validConfigYAML,
	},
}

func TestGetGCSConfig(t *testing.T) {
	for _, tc := range []struct {
		name       string
		path       string
		fake       *fakeGCSReaderFactory
		wantError  bool
		wantConfig *Config
	}{
		{
			name:       "valid and present config",
			path:       "gs://path/to/my/config.yaml",
			fake:       validFakeFactory,
			wantConfig: validConfig,
		}, {
			name:      "bad path",
			path:      "gs://path/to/nowhere.yaml",
			fake:      validFakeFactory,
			wantError: true,
		}, {
			name: "bad config",
			path: "gs://path/to/my/config.yaml",
			fake: &fakeGCSReaderFactory{
				data: map[string]string{
					"gs://path/to/my/config.yaml": `blahBADdata`,
				},
			},
			wantError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotConfig, err := getGCSConfig(context.Background(), tc.fake, tc.path)
			if err != nil {
				if tc.wantError {
					t.Logf("got expected error: %v", err)
					return
				}
				t.Fatalf("getGCSConfig(%q) failed: %v", tc.path, err)
			}

			if tc.wantError {
				t.Fatalf("getGCSConfig(%q) succeeded unexpectedly: %v", tc.path, err)
			}

			if diff := cmp.Diff(tc.wantConfig, gotConfig); diff != "" {
				t.Fatalf("getGCSConfig(%q) produced unexpected Config diff: (want- got+)\n%s", tc.path, diff)
			}
		})
	}
}

func TestGetSecretRef(t *testing.T) {
	for _, tc := range []struct {
		name      string
		parent    map[string]interface{}
		fieldName string
		wantRef   string
		wantErr   bool
	}{
		{
			name: "happy path",
			parent: map[string]interface{}{
				"mySecret": map[interface{}]interface{}{
					string(secretRef): string("bar"),
				},
			},
			fieldName: "mySecret",
			wantRef:   "bar",
		}, {
			name: "bad field name",
			parent: map[string]interface{}{
				"mySecret": map[interface{}]interface{}{
					string(secretRef): string("bar"),
				},
			},
			fieldName: "otherSecret",
			wantErr:   true,
		}, {
			name: "value is not a map",
			parent: map[string]interface{}{
				"mySecret": 404,
			},
			fieldName: "mySecret",
			wantErr:   true,
		}, {
			name: "not secret ref subfield",
			parent: map[string]interface{}{
				"mySecret": map[interface{}]interface{}{
					string("blah"): string("blah"),
				},
			},
			fieldName: "mySecret",
			wantErr:   true,
		}, {
			name: "secret ref is not a string",
			parent: map[string]interface{}{
				"mySecret": map[interface{}]interface{}{
					string(secretRef): map[interface{}]interface{}{"foo": "bar"},
				},
			},
			fieldName: "mySecret",
			wantErr:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotRef, err := GetSecretRef(tc.parent, tc.fieldName)
			if err != nil {
				if tc.wantErr {
					t.Logf("got expected error: %v", err)
					return
				}
				t.Fatalf("unexpected error: %v", err)
			}

			if gotRef != tc.wantRef {
				t.Errorf("GetSecretRef returned %q, want %q", gotRef, tc.wantRef)
			}
		})
	}
}
