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
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
)

func TestMakeCELPredicate(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name      string
		filter    string
		build     *cbpb.Build
		wantMatch bool
	}{
		{
			name:      "id match",
			filter:    `build.id == "abc"`,
			build:     &cbpb.Build{Id: "abc"},
			wantMatch: true,
		}, {
			name:      "id mismatch",
			filter:    `build.id == "abc"`,
			build:     &cbpb.Build{Id: "def"},
			wantMatch: false,
		}, {
			name:      "status match",
			filter:    "build.status ==Build.Status.SUCCESS",
			build:     &cbpb.Build{Id: "xyz", Status: cbpb.Build_SUCCESS},
			wantMatch: true,
		}, {
			name:      "status mismatch",
			filter:    "build.status == Build.Status.FAILURE",
			build:     &cbpb.Build{Id: "zyx", Status: cbpb.Build_WORKING},
			wantMatch: false,
		}, {
			name:      "trigger ID match",
			filter:    `build.build_trigger_id == "some-id"`,
			build:     &cbpb.Build{BuildTriggerId: "some-id"},
			wantMatch: true,
		}, {
			name:      "trigger ID mismatch",
			filter:    `build.build_trigger_id == "other-id"`,
			build:     &cbpb.Build{BuildTriggerId: "blah-id"},
			wantMatch: false,
		}, {
			name:      "complex filter match",
			filter:    `build.build_trigger_id == "trigger-id" && build.status == Build.Status.SUCCESS && "blah" in build.tags`,
			build:     &cbpb.Build{BuildTriggerId: "trigger-id", Status: cbpb.Build_SUCCESS, Tags: []string{"blah"}},
			wantMatch: true,
		}, {
			name:      "complex filter mismatch",
			filter:    `build.build_trigger_id == "trigger-id" && build.status == Build.Status.SUCCESS && size(build.tags) == 2 && "bar" in build.tags`,
			build:     &cbpb.Build{BuildTriggerId: "trigger-id", Status: cbpb.Build_SUCCESS, Tags: []string{"foo", "baz"}},
			wantMatch: false,
		}, {
			name:      "substitution match",
			filter:    `"key1" in build.substitutions && build.substitutions["key2"] == "val2"`,
			build:     &cbpb.Build{Substitutions: map[string]string{"key1": "val1", "key2": "val2"}},
			wantMatch: true,
		}, {
			name:      "images match",
			filter:    `"gcr.io/example/image-baz" in build.images`,
			build:     &cbpb.Build{Images: []string{"gcr.io/example/image-foo", "gcr.io/example/image-bar", "gcr.io/example/image-baz"}},
			wantMatch: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pred, err := MakeCELPredicate(tc.filter)
			if err != nil {
				t.Fatalf("MakeCELProgram(%q): %v", tc.filter, err)
			}

			if pred.Apply(ctx, tc.build) != tc.wantMatch {
				t.Errorf("CELPredicate(%+v) != %v", tc.build, tc.wantMatch)
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
apiVersion: cloud-build-notifiers/v1alpha1
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
	APIVersion: "cloud-build-notifiers/v1alpha1",
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
		Secrets: []*Secret{{
			LocalName:    "some-secret",
			ResourceName: "projects/my-project/secrets/my-secret/versions/latest",
		}},
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

func TestAddUTMParams(t *testing.T) {
	const defaultURL = "https://console.cloud.google.com/cloud-build/builds/some-build-id-here?project=12345"
	for _, tc := range []struct {
		name       string
		origURL    string
		medium     UTMMedium
		wantParams map[string][]string // Order does not matter for the values list - we use SortSlices below.
	}{
		{
			name:    "url with no params",
			origURL: "https://console.cloud.google.com/cloud-build/builds/some-build-id-here",
			medium:  EmailMedium,
			wantParams: map[string][]string{
				"utm_campaign": {"google-cloud-build-notifiers"},
				"utm_medium":   {string(EmailMedium)},
				"utm_source":   {"google-cloud-build"},
			},
		}, {
			name:    "default-like url",
			origURL: defaultURL,
			medium:  ChatMedium,
			wantParams: map[string][]string{
				"utm_campaign": {"google-cloud-build-notifiers"},
				"utm_medium":   {string(ChatMedium)},
				"utm_source":   {"google-cloud-build"},
				"project":      {"12345"},
			},
		}, {
			name: "url with with existing utm params",
			// Note that these param keys are not sorted.
			origURL: defaultURL + "&utm_campaign=blah&utm_source=do%20not%20care&utm_medium=foobar",
			medium:  HTTPMedium,
			wantParams: map[string][]string{
				"utm_campaign": {"google-cloud-build-notifiers", "blah"},
				"utm_medium":   {string(HTTPMedium), "foobar"},
				"utm_source":   {"google-cloud-build", "do not care"},
				"project":      {"12345"},
			},
		}, {
			name:    "other medium",
			origURL: defaultURL,
			medium:  OtherMedium,
			wantParams: map[string][]string{
				"utm_campaign": {"google-cloud-build-notifiers"},
				"utm_medium":   {string(OtherMedium)},
				"utm_source":   {"google-cloud-build"},
				"project":      {"12345"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newURL, err := AddUTMParams(tc.origURL, tc.medium)
			if err != nil {
				t.Fatalf("AddUTMParams(%q, %q) failed unexpectedly: %v", tc.origURL, tc.medium, err)
			}

			gotURL, err := url.Parse(newURL)
			if err != nil {
				t.Fatalf("url.Parse(%q) failed unexpectedly: %v", newURL, err)
			}

			less := func(a, b string) bool {
				return strings.Compare(a, b) < 0
			}

			for key, vals := range tc.wantParams {
				if diff := cmp.Diff(vals, gotURL.Query()[key], cmpopts.SortSlices(less)); diff != "" {
					t.Errorf("unexpected diff in values for key %q:\n%s", key, diff)
				}
			}
		})
	}
}

func TestAddUTMParamsErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		origURL string
		medium  UTMMedium
	}{{
		name:    "bad original url",
		origURL: "https://not a valid url example.com",
		medium:  OtherMedium,
	}, {
		name:    "bad encoding escape",
		origURL: "https://example.com/foo?project=12345%",
		medium:  OtherMedium,
	}, {
		name:    "coerced medium",
		origURL: "https://example.com/bar?project=12345",
		medium:  UTMMedium("gotcha"),
	}} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := AddUTMParams(tc.origURL, tc.medium)
			if err == nil {
				t.Errorf("AddUTMParams(%q, %q) succeeded unexpectedly: %v", tc.origURL, tc.medium, err)
			}
			t.Logf("AddUTMParams(%q, %q) got expected error: %v", tc.origURL, tc.medium, err)
		})
	}
}

func TestValidateConfig(t *testing.T) {
	// Config setup.
	var badAPIVersion Config
	badAPIVersion = *validConfig
	badAPIVersion.APIVersion = "something-not-correct"
	if badAPIVersion == *validConfig {
		t.Fatal("sanity check failed")
	}

	for _, tc := range []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg:  validConfig,
		}, {
			name:    "bad `apiVersion`",
			cfg:     &badAPIVersion,
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConfig(tc.cfg)
			if err != nil {
				if !tc.wantErr {
					t.Fatalf("validateConfig(%v) got unexpected error: %v", tc.cfg, err)
				} else {
					t.Logf("got expected error: %v", err)
					return
				}
			}

			if tc.wantErr {
				t.Fatalf("validateConfig(%v) unexpectedly succeeded", tc.cfg)
			}
		})
	}
}
