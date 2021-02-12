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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/storage"
	log "github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	smpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
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
	allowedYAMLAPIVersions = map[string]bool{
		"cloud-build-notifiers/v1": true,
	}
)

// Flags.
var (
	smoketest  = flag.Bool("smoketest", false, "If true, Main will simply log the notifier type and exit.")
	setupCheck = flag.Bool("setup_check", false, "If true, the configuration YAML is read from stdin and notifier.SetUp is called in a faked-out way. The smoketest flag takes priority over this one.")
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
	Filter        string                 `yaml:"filter"`
	Delivery      map[string]interface{} `yaml:"delivery"`
	Substitutions map[string]string      `yaml:"substitutions"`
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

// Copied from https://cloud.google.com/run/docs/tutorials/pubsub#looking_at_the_code.
type pubSubPushMessage struct {
	Data        []byte `json:"data,omitempty"`
	ID          string `json:"id"`
	PublishTime string `json:"publishTime"`
}

type pubSubPushWrapper struct {
	Message      pubSubPushMessage
	Subscription string `json:"subscription"`
}

// Notifier is the interface type that users should implement for usage in Cloud Build notifiers.
type Notifier interface {
	SetUp(context.Context, *Config, SecretGetter, BindingResolver) error
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
	// TODO(ljr): Refactor/separate this flagged logic from the main logic via a Main/doMain refactor.
	ctx := context.Background()

	if !flag.Parsed() {
		flag.Parse()
	}

	if *smoketest {
		log.V(0).Infof("notifier smoketest: %T", notifier)
		return nil
	}

	if *setupCheck {
		log.V(2).Info("starting setup check")
		cfg, err := decodeConfig(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to decode YAML config from stdin: %w", err)
		}

		if out, err := yaml.Marshal(cfg); err != nil {
			log.Warningf("failed to re-encode config YAML: %v", err)
		} else {
			log.V(2).Infof("got re-encoded YAML from stdin:\n%s", string(out))
		}

		if err := validateConfig(cfg); err != nil {
			return fmt.Errorf("failed to validate config during setup check: %w", err)
		}

		br, err := newResolver(cfg)
		if err != nil {
			return fmt.Errorf("failed to create BindingResolver during setup check: %w", err)
		}

		if err := notifier.SetUp(ctx, cfg, new(setupCheckSecretGetter), br); err != nil {
			return fmt.Errorf("failed to run notifier.SetUp during setup check: %w", err)
		}

		log.V(2).Infof("setup check successful")
		return nil
	}

	cfgPath, ok := GetEnv("CONFIG_PATH")
	if !ok {
		return errors.New("expected CONFIG_PATH to be non-empty")
	}

	sc, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create new GCS client: %w", err)
	}
	defer sc.Close()

	smc, err := secretmanager.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create new SecretManager client: %w", err)
	}
	defer smc.Close()

	cfg, err := getGCSConfig(ctx, &actualGCSReaderFactory{sc}, cfgPath)
	if err != nil {
		return fmt.Errorf("failed to get config from GCS: %w", err)
	}
	if err := validateConfig(cfg); err != nil {
		return fmt.Errorf("got invalid config from path %q: %w", cfgPath, err)
	}
	log.V(2).Infof("got config from GCS (%q): %+v\n", cfgPath, cfg)

	sm := &actualSecretManager{client: smc}

	br, err := newResolver(cfg)
	if err != nil {
		return fmt.Errorf("failed to construct a binding resolver: %v", err)
	}

	if err := notifier.SetUp(ctx, cfg, sm, br); err != nil {
		return fmt.Errorf("failed to call SetUp on notifier: %w", err)
	}

	_, ignoreBadMessages := GetEnv("IGNORE_BAD_MESSAGES")

	log.V(2).Infoln("starting HTTP server...")

	// Our Pub/Sub push receiver.
	http.HandleFunc("/", newReceiver(notifier, &receiverParams{ignoreBadMessages}))

	// An auxilliary, healthz-style receiver.
	// You can call this endpoint using the curl command here:
	// https://cloud.google.com/run/docs/triggering/https-request#creating_private_services.
	startTime := time.Now()
	http.HandleFunc("/helloz", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "Greetings from a Google Cloud Build notifier: %T!\nStart Time: %s\nCurrent Time: %s\n",
			notifier, startTime.Format(time.RFC1123), time.Now().Format(time.RFC1123))
	})

	var port string
	if p, ok := GetEnv("PORT"); ok {
		port = p
	} else {
		log.Warningf("PORT environment variable was not present, using %s instead", defaultHTTPPort)
		port = defaultHTTPPort
	}

	// Block on the HTTP's health.
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
	// TODO(ljr): Do we want any sort of timed cache here?
}

func (a *actualSecretManager) GetSecret(ctx context.Context, name string) (string, error) {
	// See https://github.com/GoogleCloudPlatform/golang-samples/blob/master/secretmanager/access_secret_version.go# for an example usage.
	res, err := a.client.AccessSecretVersion(ctx, &smpb.AccessSecretVersionRequest{Name: name})
	if err != nil {
		return "", fmt.Errorf("failed to get secret named %q: %w", name, err)
	}

	return string(res.GetPayload().GetData()), nil
}

// setupCheckSecretGetter is a faked-out SecretGetter that is only used by the setup check functionality in Main.
type setupCheckSecretGetter struct{}

func (c *setupCheckSecretGetter) GetSecret(_ context.Context, name string) (string, error) {
	return fmt.Sprintf("[SECRET VALUE FOR %q]", name), nil
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
		return nil, fmt.Errorf("failed to get reader for (bucket=%q, object=%q): %w", bucket, object, err)
	}
	defer r.Close()

	cfg, err := decodeConfig(r)
	if err != nil {
		return nil, fmt.Errorf("failed to parse configuration from YAML at %q: %w", path, err)
	}

	return cfg, nil
}

func decodeConfig(r io.Reader) (*Config, error) {
	cfg := new(Config)
	dcd := yaml.NewDecoder(r)
	dcd.SetStrict(true)
	return cfg, dcd.Decode(cfg)
}

// validateConfig checks the following (or errors):
// - apiVersion is one of allowedYAMLAPIVersions.
// - user substitution names match the subNamePattern regexp.
func validateConfig(cfg *Config) error {
	if allowed := allowedYAMLAPIVersions[cfg.APIVersion]; !allowed {
		return fmt.Errorf("expected `apiVersion` %q to be one of the following:\n%v",
			cfg.APIVersion, allowedYAMLAPIVersions)
	}

	if cfg.Spec == nil {
		return errors.New("expected config.spec to be present")
	}

	if cfg.Spec.Notification == nil {
		return errors.New("expected config.spec.notification to be present")
	}

	for n := range cfg.Spec.Notification.Substitutions {
		if !subNamePattern.MatchString(n) {
			return fmt.Errorf("expected user-defined substitution %q to match pattern %v", n, subNamePattern)
		}
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
		return nil, fmt.Errorf("failed to create a CEL env: %w", err)
	}

	ast, issues := env.Compile(filter)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("failed to compile CEL filter %q: %w", filter, issues.Err())
	}

	if !proto.Equal(ast.ResultType(), decls.Bool) {
		return nil, fmt.Errorf("expected CEL filter %q to have a boolean result type, but was %v", filter, ast.ResultType())
	}

	prg, err := env.Program(ast, cel.EvalOptions(cel.OptOptimize))
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL program from filter %q: %w", filter, err)
	}

	return &CELPredicate{prg}, nil
}

// GetEnv fetches, logs, and returns the given environment variable. The returned boolean is true iff the value is non-empty.
func GetEnv(name string) (string, bool) {
	val := os.Getenv(name)
	if val == "" {
		log.V(2).Infof("env var %q is empty", name)
	} else {
		log.V(2).Infof("env var %q is %q", name, val)
	}
	return val, val != ""
}

type receiverParams struct {
	ignoreBadMessages bool
}

// newReceiver returns a Pub/Sub push HTTP receiving http.HandlerFunc that calls the given notifier.
func newReceiver(notifier Notifier, params *receiverParams) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var pspw pubSubPushWrapper
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Errorf("failed to read request message: %v", err)
			http.Error(w, "Bad request body", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &pspw); err != nil {
			log.Errorf("failed to unmarshal body %q: %v", body, err)
			http.Error(w, "Bad pubsub.Message JSON", http.StatusBadRequest)
			return
		}

		log.V(2).Infof("got PubSub message with ID %q from subscription %q", pspw.Message.ID, pspw.Subscription)

		build := new(cbpb.Build)
		// Be as lenient as possible in unmarshalling.
		// `Unmarshal` will fail if we get a payload with a field that is unknown to the current proto version unless `DiscardUnknown` is set.
		uo := protojson.UnmarshalOptions{
			AllowPartial:   true,
			DiscardUnknown: true,
		}
		bv2 := proto.MessageV2(build)
		if err := uo.Unmarshal(pspw.Message.Data, bv2); err != nil {
			if params.ignoreBadMessages {
				log.Warningf("not attempting to handle unmarshal-able Pub/Sub message id=%q data=%q publishTime=%q which gave error: %v",
					pspw.Message.ID, string(pspw.Message.Data), pspw.Message.PublishTime, err)
				return
			}

			log.Errorf("failed to unmarshal PubSub message id=%q data=%q publishTime=%q into a Build: %v",
				pspw.Message.ID, string(pspw.Message.Data), pspw.Message.PublishTime, err)
			http.Error(w, "Bad Cloud Build Pub/Sub data", http.StatusBadRequest)
			return
		}
		build = proto.MessageV1(bv2).(*cbpb.Build)

		log.V(2).Infof("got PubSub Build payload:\n%+v\nattempting to send notification", proto.MarshalTextString(build))
		if err := notifier.SendNotification(ctx, build); err != nil {
			log.Errorf("failed to run SendNotification: %v", err)
			http.Error(w, "failed to send notification", http.StatusInternalServerError)
			return
		}

		log.V(2).Infof("acking PubSub message %q with Build payload:\n%v", pspw.Message.ID, proto.MarshalTextString(build))
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
	// StorageMedium is for Build log URLS that are sent to a storage medium (i.e. BigQuery).
	StorageMedium = "storage"
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
		return "", fmt.Errorf("failed to parse URL %q: %w", logURL, err)
	}

	// Use ParseQuery to fail if we get malformed params to start with, since it should never happen.
	vals, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", fmt.Errorf("failed to parse query from %q: %w", logURL, err)
	}

	var m string
	switch medium {
	case EmailMedium, StorageMedium, ChatMedium, HTTPMedium, OtherMedium:
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
