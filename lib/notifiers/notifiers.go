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
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/pubsub"
	secretmanager "cloud.google.com/go/secretmanager/apiv1beta1"
	"cloud.google.com/go/storage"
	log "github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	smpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1beta1"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v2"
)

const (
	cloudBuildProtoPkg = "google.devtools.cloudbuild.v1"
	cloudBuildTopic    = "cloud-builds"
	defaultHTTPPort    = "8080"
	secretRef          = "secretRef"
)

var (
	// Set of allowed notifier Config `apiVersions`.
	allowedYAMLAPIVersions = map[string]bool{"cloud-build-notifiers/v1alpha1": true}
)

// Flags.
var (
	smoketestFlag = flag.Bool("smoketest", false, "If true, Main will simply log the notifier type and exit.")
)

// Config is the common type for (YAML-based) configuration files for notifications.
type Config struct {
	APIVersion string    `yaml:"apiVersion"`
	Kind       string    `yaml:"kind"`
	Metadata   *Metadata `yaml:"metadata"`
	Spec       *Spec     `yaml:"spec"`
}

// Metadata is a KRD-compliant data container used for metadata references.
type Metadata struct {
	Name string `yaml:"name"`
}

// Spec is the data container for the fields that are relevant to the functionality of the notifier.
type Spec struct {
	Notification *Notification `yaml:"notification"`
	Secrets      []*Secret     `yaml:"secrets"`
}

// Notification is the data container for the fields that are relevant to the configuration of sending the notification.
type Notification struct {
	Filter   string                 `yaml:"filter"`
	Delivery map[string]interface{} `yaml:"delivery"`
}

// SecretConfig is the data container used in a Spec.Notification config for referencing a secret in the Spec.Secrets list.
type SecretConfig struct {
	LocalName string `yaml:"secretRef"`
}

// Secret is a data container matching the local name of a secret to its GCP SecretManager resource name.
type Secret struct {
	LocalName    string `yaml:"name"`
	ResourceName string `yaml:"value"`
}

// Notifier is the interface type that users should implement for usage in Cloud Build notifiers.
type Notifier interface {
	SetUp(context.Context, *Config, SecretGetter) error
	SendNotification(context.Context, *cbpb.Build) error
}

// SecretGetter allows for fetching secrets from some key store.
type SecretGetter interface {
	GetSecret(context.Context, string) (string, error)
}

// EventFilter is a type that can be used to filter Builds for notifications.
type EventFilter interface {
	// Apply returns true iff the EventFilter is able to execute successfully and matches the given Build.
	Apply(context.Context, *cbpb.Build) bool
}

// CELPredicate is an EventFilter that uses a CEL program to determine if
// notifications should be sent for a given Pub/Sub message.
type CELPredicate struct {
	prg cel.Program
}

// Apply returns true iff the underlying CEL program returns true for the given Build.
func (c *CELPredicate) Apply(_ context.Context, build *cbpb.Build) bool {
	out, _, err := c.prg.Eval(map[string]interface{}{"build": build})
	if err != nil {
		log.Errorf("failed to evaluate the CEL filter: %v", err)
		return false
	}

	match, ok := out.Value().(bool)
	if !ok {
		log.Errorf("failed to convert output %v of CEL filter program to a boolean: %v", out, err)
		return false
	}

	return match
}

// Main is a function that can be called by `main()` functions in notifier binaries.
func Main(notifier Notifier) error {
	if !flag.Parsed() {
		flag.Parse()
	}

	if *smoketestFlag {
		log.V(0).Infof("notifier smoketest: %T", notifier)
		return nil
	}

	projectID, ok := GetEnv("PROJECT_ID")
	if !ok {
		return errors.New("expected PROJECT_ID to be non-empty")
	}

	ctx := context.Background()

	subscriberID, ok := GetEnv("SUBSCRIBER_ID")
	if !ok {
		return errors.New("expected SUBSCRIBER_ID to be non-empty")
	}

	log.V(2).Infof("Cloud PubSub subscriber ID is: %q", subscriberID)

	cfgPath, ok := GetEnv("CONFIG_PATH")
	if !ok {
		return errors.New("expected CONFIG_PATH to be non-empty")
	}

	sc, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create new GCS client: %v", err)
	}
	defer sc.Close()

	smc, err := secretmanager.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create new SecretManager client: %v", err)
	}
	defer smc.Close()

	cfg, err := getGCSConfig(ctx, &actualGCSReaderFactory{sc}, cfgPath)
	if err != nil {
		return fmt.Errorf("failed to get config from GCS: %v", err)
	}
	if err := validateConfig(cfg); err != nil {
		return fmt.Errorf("got invalid config from path %q: %v", cfgPath, err)
	}
	log.V(2).Infof("got config from GCS (%q): %+v\n", cfgPath, cfg)

	if err := notifier.SetUp(ctx, cfg, &actualSecretManager{smc}); err != nil {
		return fmt.Errorf("failed to call SetUp on notifier: %v", err)
	}

	sub, err := NewSubscription(ctx, projectID, subscriberID)
	if err != nil {
		return fmt.Errorf("failed to create PubSub subscription: %v", err)
	}

	// We need to start up both receivers in parallel of eachother.
	// We need an HTTP handler on the given PORT to satisfy Cloud Run's health checks.
	log.V(2).Infof("starting PubSub (%q) and HTTP handlers...\n", subscriberID)

	// Run the receiver in a goroutine because `sub.Receive` blocks and so does the HTTP server below.
	go func() {
		// TODO(ljr): Any error here should probably be bubbled-up and crash the main process.
		if err := sub.Receive(ctx, NewReceiver(notifier, cfg)); err != nil {
			log.Errorf("failed to properly send notification for received PubSub message: %v", err)
		}
	}()

	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "Greetings from a Google Cloud Build notifier: %T! (Time: %s)", notifier, time.Now().String())
	})

	var port string
	if p, ok := GetEnv("PORT"); ok {
		port = p
	} else {
		log.Warningf("PORT environment variable was not present, using %s instead", defaultHTTPPort)
		port = defaultHTTPPort
	}

	// Block on the HTTP handler server while the PubSub handler is blocked in the goroutine above.
	return http.ListenAndServe(":"+port, nil)
}

type gcsReaderFactory interface {
	NewReader(ctx context.Context, bucket, object string) (io.ReadCloser, error)
}

type actualGCSReaderFactory struct {
	client *storage.Client
}

func (a *actualGCSReaderFactory) NewReader(ctx context.Context, bucket, object string) (io.ReadCloser, error) {
	return a.client.Bucket(bucket).Object(object).NewReader(ctx)
}

type actualSecretManager struct {
	client *secretmanager.Client
}

func (a *actualSecretManager) GetSecret(ctx context.Context, name string) (string, error) {
	// See https://github.com/GoogleCloudPlatform/golang-samples/blob/master/secretmanager/access_secret_version.go# for an example usage.
	res, err := a.client.AccessSecretVersion(ctx, &smpb.AccessSecretVersionRequest{Name: name})
	if err != nil {
		return "", fmt.Errorf("failed to get secret named %q: %v", name, err)
	}

	return string(res.GetPayload().GetData()), nil
}

// getGCSConfig fetches the YAML Config file from the given GCS path and returns the parsed Config.
func getGCSConfig(ctx context.Context, grf gcsReaderFactory, path string) (*Config, error) {
	if trm := strings.TrimPrefix(path, "gs://"); trm != path {
		// path started with the prefix
		path = trm
	} else {
		return nil, fmt.Errorf("expected %q to start with `gs://`", path)
	}

	split := strings.SplitN(path, "/", 2)
	log.V(2).Infof("got path split: %+v", split)
	if len(split) != 2 {
		return nil, fmt.Errorf("path has incorrect format (expected form: `[gs://]bucket/path/to/object`): %q => %s", path, strings.Join(split, ", "))
	}

	bucket, object := split[0], split[1]
	r, err := grf.NewReader(ctx, bucket, object)
	if err != nil {
		return nil, fmt.Errorf("failed to get reader for (bucket=%q, object=%q): %v", bucket, object, err)
	}
	defer r.Close()

	cfg := new(Config)
	dcd := yaml.NewDecoder(r)
	dcd.SetStrict(true)
	if err := dcd.Decode(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse configuration from YAML at %q: %v", path, err)
	}

	return cfg, nil
}

// validateConfig checks the following (or errors):
// - apiVersion is one of allowedYAMLAPIVersions.
func validateConfig(cfg *Config) error {
	if allowed := allowedYAMLAPIVersions[cfg.APIVersion]; !allowed {
		return fmt.Errorf("expected `apiVersion` %q to be one of the following:\n%v",
			cfg.APIVersion, allowedYAMLAPIVersions)
	}
	return nil
}

// MakeCELPredicate returns a CELPredicate for the given filter string of CEL code.
func MakeCELPredicate(filter string) (*CELPredicate, error) {
	env, err := cel.NewEnv(
		// Declare the `build` variable for useage in CEL programs.
		cel.Declarations(decls.NewIdent("build", decls.NewObjectType(cloudBuildProtoPkg+".Build"), nil)),
		// Register the `Build` type in the environment.
		cel.Types(new(cbpb.Build)),
		// `Container` is necessary for better (enum) scoping
		// (i.e with this, we don't need to use the fully qualified proto path in our programs).
		cel.Container(cloudBuildProtoPkg),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create a CEL env: %v", err)
	}

	ast, celErr := env.Parse(filter)
	if celErr != nil && celErr.Err() != nil {
		return nil, fmt.Errorf("failed to parse CEL filter %q: %v", filter, celErr.Err())
	}

	ast, celErr = env.Check(ast)
	if celErr != nil && celErr.Err() != nil {
		return nil, fmt.Errorf("failed to type check CEL filter %q: %v", filter, celErr.Err())
	}

	prg, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL program: %v", err)
	}

	return &CELPredicate{prg}, nil
}

// GetEnv fetches, logs, and returns the given environment variable. The returned boolean is true iff the value is non-empty.
func GetEnv(name string) (string, bool) {
	val := os.Getenv(name)
	if val == "" {
		log.Warningf("env var %q is empty", name)
	} else {
		log.V(2).Infof("env var %q is %q", name, val)
	}
	return val, val != ""
}

// NewSubscription returns a Cloud PubSub subscription for the given project and subscriber IDs.
func NewSubscription(ctx context.Context, projectID, subscriberID string) (*pubsub.Subscription, error) {
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create PubSub client: %v", err)
	}
	return client.Subscription(subscriberID), nil
}

// NewReceiver returns a PubSub receiving function that calls the given notifier, using and passing the Config as necessary.
func NewReceiver(notifier Notifier, cfg *Config) func(context.Context, *pubsub.Message) {
	return func(ctx context.Context, msg *pubsub.Message) {
		log.V(2).Infof("got PubSub message with ID: %q", msg.ID)

		build := new(cbpb.Build)
		// Be as lenient as possible in unmarshalling.
		// `Unmarshal` will fail if we get a payload with a field that is unknown to the current proto version unless `DiscardUnknown` is set.
		uo := protojson.UnmarshalOptions{
			AllowPartial:   true,
			DiscardUnknown: true,
		}
		bv2 := proto.MessageV2(build)
		if err := uo.Unmarshal(msg.Data, bv2); err != nil {
			log.Errorf("failed to unmarshal PubSub message %q into a Build: %v", msg.ID, err)
			msg.Nack()
			return
		}
		build = proto.MessageV1(bv2).(*cbpb.Build)

		log.V(2).Infof("got PubSub Build payload:\n%+v\nattempting to send notification", proto.MarshalTextString(build))
		if err := notifier.SendNotification(ctx, build); err != nil {
			log.Errorf("failed to run SendNotification: %v", err)
			msg.Nack()
			return
		}

		log.V(2).Infof("acking PubSub message %q with Build payload:\n%v", msg.ID, proto.MarshalTextString(build))
		msg.Ack()
	}
}

// GetSecretRef is a helper function for getting a Secret's local reference name from the given config.
func GetSecretRef(config map[string]interface{}, fieldName string) (string, error) {
	field, ok := config[fieldName]
	if !ok {
		return "", fmt.Errorf("field name %q not present in notification config %v", fieldName, config)
	}
	m, ok := field.(map[interface{}]interface{})
	if !ok {
		return "", fmt.Errorf("expected secret field %q to be a map[interface{}]interface{} object", fieldName)
	}
	ref, ok := m[secretRef]
	if !ok {
		return "", fmt.Errorf("expected field %q to be of the form `secretRef: <some-ref>`", fieldName)
	}
	sRef, ok := ref.(string)
	if !ok {
		return "", fmt.Errorf("expected field %q of parent %q to have a string value", secretRef, fieldName)
	}

	return sRef, nil
}

// FindSecretResourceName is a helper function that returns the Secret's resource name that is associated with the given local reference name.
func FindSecretResourceName(secrets []*Secret, ref string) (string, error) {
	for _, s := range secrets {
		if s.LocalName == ref {
			return s.ResourceName, nil
		}
	}
	return "", fmt.Errorf("failed to find Secret with reference name %q in the given secret list", ref)
}

// UTMMedium is an enum that corresponds to a strict set of values for `utm_medium`.
type UTMMedium string

const (
	// EmailMedium is for Build log URLs that are sent via email.
	EmailMedium UTMMedium = "email"
	// ChatMedium is for Build log URLs that are sent over chat applications.
	ChatMedium = "chat"
	// HTTPMedium is for Build log URLs that are sent over HTTP(S) communication (that does not belong to one of the other mediums).
	HTTPMedium = "http"
	// OtherMedium is for Build log URLs that sent are over a medium that does not correspond to one of the above mediums.
	OtherMedium = "other"
)

// AddUTMParams adds UTM campaign tracking parameters to the given Build log URL and returns the new version.
// The UTM parameters are added to any existing ones, so any existing params will not be ovewritten.
func AddUTMParams(logURL string, medium UTMMedium) (string, error) {
	u, err := url.Parse(logURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL %q: %v", logURL, err)
	}

	// Use ParseQuery to fail if we get malformed params to start with, since it should never happen.
	vals, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", fmt.Errorf("failed to parse query from %q: %v", logURL, err)
	}

	var m string
	switch medium {
	case EmailMedium, ChatMedium, HTTPMedium, OtherMedium:
		m = string(medium)
	default:
		return "", fmt.Errorf("unknown UTM medium: %q", medium)
	}

	// Use `Add` instead of `Set` so we don't override any existing params.
	vals.Add("utm_campaign", "google-cloud-build-notifiers")
	vals.Add("utm_medium", m)
	vals.Add("utm_source", "google-cloud-build")

	u.RawQuery = vals.Encode()

	return u.String(), nil
}
