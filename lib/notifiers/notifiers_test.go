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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"

	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func convertToTimestamp(t *testing.T, datetime string) *timestamppb.Timestamp {
	timestamp, err := time.Parse(time.RFC3339, datetime)
	if err != nil {
		t.Fatalf("Failed to parse datetime string: %v", err)
	}
	ppbtimestamp, err := ptypes.TimestampProto(timestamp)
	if err != nil {
		t.Fatalf("Failed to parse timestamp: %v", err)
	}
	return ppbtimestamp
}

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
			filter:    "build.status == Build.Status.SUCCESS",
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
		}, {
			name:      "status list match",
			filter:    `build.status in [Build.Status.SUCCESS, Build.Status.FAILURE]`,
			build:     &cbpb.Build{Status: cbpb.Build_FAILURE},
			wantMatch: true,
		}, {
			name:      "substitutions mismatch",
			filter:    `"DINNER" in build.substitutions && build.substitutions["DINNER"] == "PIZZA"`,
			build:     &cbpb.Build{Substitutions: map[string]string{"DESSERT": "CANNOLI"}},
			wantMatch: false,
		}, {
			name:      "before timestamp",
			filter:    `timestamp("2020-01-01T10:00:20.021-05:00") <= build.start_time`,
			build:     &cbpb.Build{StartTime: convertToTimestamp(t, "2019-01-01T10:00:20.021-05:00")},
			wantMatch: false,
		},
		{
			name:      "after timestamp",
			filter:    `timestamp("1972-01-01T10:00:20.021-05:00") < build.start_time`,
			build:     &cbpb.Build{StartTime: convertToTimestamp(t, "2019-01-01T10:00:20.000-05:00")},
			wantMatch: true,
		},
		{
			name:      "before integer year",
			filter:    `timestamp("2019-01-01T10:00:20.000-05:00") > build.start_time`,
			build:     &cbpb.Build{StartTime: convertToTimestamp(t, "2018-01-01T10:00:20.000-05:00")},
			wantMatch: true,
		},
		{
			name:      "after integer year",
			filter:    `timestamp("2019-01-01T10:00:20.000-05:00") < build.start_time`,
			build:     &cbpb.Build{StartTime: convertToTimestamp(t, "2020-01-01T10:00:20.000-05:00")},
			wantMatch: true,
		},
		{
			name:      "specific day match",
			filter:    `timestamp("2019-07-24T00:00:00.000-05:00") <= build.start_time && build.start_time < timestamp("2019-07-25T00:00:00.000-05:00")`,
			build:     &cbpb.Build{StartTime: convertToTimestamp(t, "2019-07-24T12:00:00.000-00:00")},
			wantMatch: true,
		},
		{
			name:      "not specific day match",
			filter:    `timestamp("2020-07-24T00:00:00.000-00:00") <= build.start_time && build.start_time < timestamp("2020-07-25T00:00:00.000-00:00")`,
			build:     &cbpb.Build{StartTime: convertToTimestamp(t, "2019-07-23T00:00:00.000-05:00")},
			wantMatch: false,
		},
		{
			name:      "first of the month",
			filter:    `build.start_time.getDayOfMonth() == 0`,
			build:     &cbpb.Build{StartTime: convertToTimestamp(t, "2019-07-02T00:00:00.000-00:00")},
			wantMatch: false,
		},
		{
			name:      "first of the month match",
			filter:    `build.start_time.getDayOfMonth()==0`,
			build:     &cbpb.Build{StartTime: convertToTimestamp(t, "2019-07-01T12:00:00.000-00:00")},
			wantMatch: true,
		},
		{
			name:      "build at least five minutes",
			filter:    `build.finish_time - build.start_time >= duration("300s")`,
			build:     &cbpb.Build{StartTime: convertToTimestamp(t, "2019-07-01T12:00:00.000-00:00"), FinishTime: convertToTimestamp(t, "2019-07-01T12:10:00.000-00:00")},
			wantMatch: true,
		},
		{
			name:      "build shorter than five minutes",
			filter:    `build.finish_time - build.start_time < duration("300s")`,
			build:     &cbpb.Build{StartTime: convertToTimestamp(t, "2019-07-01T12:00:00.000-00:00"), FinishTime: convertToTimestamp(t, "2019-07-01T12:00:03.000-00:00")},
			wantMatch: true,
		}, {
			name:      "complex filter with regexp via `matches`",
			filter:    `build.status in [Build.Status.FAILURE, Build.Status.TIMEOUT] && build.substitutions["TAG_NAME"].matches("^v\\d{1}\\.\\d{1}\\.\\d{3}$")`,
			build:     &cbpb.Build{Status: cbpb.Build_TIMEOUT, Substitutions: map[string]string{"TAG_NAME": "v1.2.003"}},
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

func TestMakeCELPredicateErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		filter string
	}{{
		name:   "bad variable",
		filter: `event.id == "foo"`,
	}, {
		name:   "bad enum usage",
		filter: `build.status == "SUCCESS"`,
	}, {
		name:   "unknown field",
		filter: `build.salad == "kale"`,
	}, {
		name:   "bad result type",
		filter: `build.id`,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := MakeCELPredicate(tc.filter); err == nil {
				t.Errorf("MakeCELPredicate(%q) unexpectedly succeeded", tc.filter)
			} else {
				t.Logf("MakeCELPredicate(%q) got expected error: %v", tc.filter, err)
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

// It's annoying to update this config since YAML requires spaces but Go likes tabs.
// Just keep everything at tabs and then replace accordingly.
const validConfigYAMLWithTabs = `
apiVersion: cloud-build-notifiers/v1
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
    substitutions:
      _SOME_SUBST: $(build['_SOME_SUBST'])
      _SOME_SECRET: $(secrets['some-secret'])
  secrets:
    - name: some-secret
      value: projects/my-project/secrets/my-secret/versions/latest
`

var validConfig = &Config{
	APIVersion: "cloud-build-notifiers/v1",
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
			Substitutions: map[string]string{
				"_SOME_SUBST":  "$(build['_SOME_SUBST'])",
				"_SOME_SECRET": "$(secrets['some-secret'])",
			},
		},
		Secrets: []*Secret{{
			LocalName:    "some-secret",
			ResourceName: "projects/my-project/secrets/my-secret/versions/latest",
		}},
	},
}

func TestGetGCSConfig(t *testing.T) {
	validYAML := strings.ReplaceAll(validConfigYAMLWithTabs, "\t", "    " /* 4 spaces */)
	validFakeFactory := &fakeGCSReaderFactory{
		data: map[string]string{
			"gs://path/to/my/config.yaml": validYAML,
		},
	}

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
	for _, tc := range []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg:  validConfig,
		}, {
			name: "bad `apiVersion`",
			cfg: &Config{
				APIVersion: "some-bad-api-version",
				Spec:       &Spec{Notification: &Notification{}},
			},
			wantErr: true,
		}, {
			name: "no spec",
			cfg: &Config{
				APIVersion: "cloud-build-notifiers/v1",
			},
			wantErr: true,
		}, {
			name: "no spec.notification",
			cfg: &Config{
				APIVersion: "cloud-build-notifiers/v1",
				Spec:       &Spec{},
			},
			wantErr: true,
		}, {
			name: "subst name with no underscore",
			cfg: &Config{
				APIVersion: "cloud-build-notifiers/v1",
				Spec: &Spec{
					Notification: &Notification{
						Substitutions: map[string]string{
							"FOO": "$(build.id)",
						},
					},
				},
			},
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

type fakeNotifier struct {
	notifs chan *cbpb.Build
}

func (f *fakeNotifier) SetUp(_ context.Context, _ *Config, _ SecretGetter, _ BindingResolver) error {
	// Not currently called by any test.
	return nil
}

func (f *fakeNotifier) SendNotification(_ context.Context, build *cbpb.Build) error {
	f.notifs <- proto.Clone(build).(*cbpb.Build)
	return nil
}

func TestNewReceiver(t *testing.T) {
	const projectID = "some-project-id"
	sentBuild := &cbpb.Build{
		ProjectId:     projectID,
		Id:            "some-build-id",
		Status:        cbpb.Build_FAILURE,
		Substitutions: map[string]string{"foo": "bar"},
		Tags:          []string{t.Name()},
		Images:        []string{"gcr.io/example/image"},
	}
	sentBuildV2 := proto.MessageV2(sentBuild)
	sentJSON, err := protojson.Marshal(sentBuildV2)
	if err != nil {
		t.Fatal(err)
	}

	pspw := &pubSubPushWrapper{
		Message:      pubSubPushMessage{Data: sentJSON, ID: "id-do-not-care"},
		Subscription: "subscriber-do-not-care",
	}

	j, err := json.Marshal(pspw)
	if err != nil {
		t.Fatal(err)
	}

	bc := make(chan *cbpb.Build, 1)
	fn := &fakeNotifier{notifs: bc}

	handler := newReceiver(fn, &receiverParams{})
	req := httptest.NewRequest(http.MethodPost, "http://notifer.example.com/", bytes.NewBuffer(j))
	w := httptest.NewRecorder()

	handler(w, req)
	if s := w.Result().StatusCode; s != http.StatusOK {
		t.Errorf("result.StatusCode = %d, expected %d", s, http.StatusOK)
	}

	// Wait for our fakeNotifier to send us a Build.
	var gotBuild *cbpb.Build
	select {
	case b := <-bc:
		gotBuild = b
	case <-time.After(10 * time.Second):
		t.Fatal("failed to received a Build from the notifier before the timeout")
	}

	if diff := cmp.Diff(sentBuild, gotBuild, protocmp.Transform()); diff != "" {
		t.Errorf("unexpected difference between published Build and received Build:\n%s", diff)
	}
}

type errNotifier struct {
	err error
}

func (n *errNotifier) SetUp(_ context.Context, _ *Config, _ SecretGetter, _ BindingResolver) error {
	return nil
}
func (n *errNotifier) SendNotification(_ context.Context, _ *cbpb.Build) error {
	return n.err
}

func wrapperToBuffer(t *testing.T, w *pubSubPushWrapper) *bytes.Buffer {
	t.Helper()
	j, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewBuffer(j)
}

func buildToBuffer(t *testing.T, b *cbpb.Build) *bytes.Buffer {
	t.Helper()
	b2 := proto.MessageV2(b)
	j, err := protojson.Marshal(b2)
	if err != nil {
		t.Fatal(err)
	}

	return wrapperToBuffer(t, &pubSubPushWrapper{
		Subscription: "subscriber-does-not-matter",
		Message:      pubSubPushMessage{ID: "id-does-not-matter", Data: j},
	})
}

func TestNewReceiverError(t *testing.T) {
	for _, tc := range []struct {
		name     string
		body     *bytes.Buffer
		sendErr  error
		wantCode int
	}{
		{
			name:     "empty body",
			body:     new(bytes.Buffer),
			wantCode: http.StatusBadRequest,
		}, {
			name:     "empty wrapper",
			body:     wrapperToBuffer(t, &pubSubPushWrapper{}),
			wantCode: http.StatusBadRequest,
		}, {
			name:     "bad data",
			body:     wrapperToBuffer(t, &pubSubPushWrapper{Message: pubSubPushMessage{Data: []byte(`#corrupted#`)}}),
			wantCode: http.StatusBadRequest,
		}, {
			name:     "send notification error",
			body:     buildToBuffer(t, new(cbpb.Build)),
			sendErr:  errors.New("failed to reticulate splines"),
			wantCode: http.StatusInternalServerError,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := newReceiver(&errNotifier{tc.sendErr}, &receiverParams{})

			req := httptest.NewRequest(http.MethodPost, "http://notifer.example.com/", tc.body)
			w := httptest.NewRecorder()

			handler(w, req)

			if s := w.Result().StatusCode; s != tc.wantCode {
				t.Errorf("result.StatusCode = %d, expected %d", s, tc.wantCode)
			}
		})
	}
}

type fatalNotifier struct {
	t *testing.T
}

func (n *fatalNotifier) SetUp(_ context.Context, _ *Config, _ SecretGetter, _ BindingResolver) error {
	return nil
}
func (n *fatalNotifier) SendNotification(_ context.Context, b *cbpb.Build) error {
	n.t.Helper()
	n.t.Fatalf("should not have been called; was called with build: %v", b)
	return nil
}

func TestReceiverWithIgnoredBadMessage(t *testing.T) {
	const projectID = "some-project-id"
	pspw := &pubSubPushWrapper{
		Message:      pubSubPushMessage{Data: []byte("#bA4Fx"), ID: "id-do-not-care"},
		Subscription: "subscriber-do-not-care",
	}

	j, err := json.Marshal(pspw)
	if err != nil {
		t.Fatal(err)
	}

	handler := newReceiver(&fatalNotifier{t}, &receiverParams{ignoreBadMessages: true})
	req := httptest.NewRequest(http.MethodPost, "http://notifer.example.com/", bytes.NewBuffer(j))
	w := httptest.NewRecorder()

	handler(w, req)
	if s := w.Result().StatusCode; s != http.StatusOK {
		t.Errorf("result.StatusCode = %d, expected %d", s, http.StatusOK)
	}
}
