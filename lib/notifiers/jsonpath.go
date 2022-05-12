package notifiers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"

	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
	"k8s.io/client-go/third_party/forked/golang/template"
	"k8s.io/client-go/util/jsonpath"
)

// BindingResolver is an object that given a Build and a way to get secrets, returns all bound substitutions from the
// notifier configuration.
type BindingResolver interface {
	Resolve(context.Context, SecretGetter, *cbpb.Build) (map[string]string, error)
}

type inputAndJSONPath struct {
	j *jsonpath.JSONPath // The JSONPath that parsed that path.
	p string             // The user-provided path.
}

type jpResolver struct {
	mtx sync.RWMutex
	jps map[string]*inputAndJSONPath // Map of _SOME_SUBST_NAME => its inputAndJSONPath.
	cfg *Config
}

func newResolver(cfg *Config) (BindingResolver, error) {
	jps := map[string]*inputAndJSONPath{}
	for name, path := range cfg.Spec.Notification.Params {
		p, err := makeJSONPath(path)
		if err != nil {
			return nil, fmt.Errorf("failed to derive substitution path from %q: %v", path, err)
		}
		j := jsonpath.New(name).AllowMissingKeys(false)
		if err := j.Parse(p); err != nil {
			return nil, fmt.Errorf("failed to parse JSONPath expression from %q: %v", path, err)
		}

		jps[name] = &inputAndJSONPath{
			j: j,
			p: path, // Use the user-provided path so error messages are easier to understand.
		}
	}
	return &jpResolver{
		jps: jps,
		cfg: cfg,
	}, nil
}

func (j *jpResolver) Resolve(ctx context.Context, sg SecretGetter, build *cbpb.Build) (map[string]string, error) {
	j.mtx.RLock()
	defer j.mtx.RUnlock()

	// Use a "JSON" payload here since a struct would have export-field issues
	// based on the lowercase names.
	pld := map[string]interface{}{
		"build": build,
	}

	ret := map[string]string{}
	for name, jp := range j.jps {
		fullResults, err := jp.j.FindResults(pld)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %q with path %q from payload: %v", name, jp.p, err)
		}

		if len(fullResults) == 0 {
			return nil, fmt.Errorf("failed to get JSONPath query results for %q with path %q", name, jp.p)
		}

		buf := new(bytes.Buffer)
		for _, r := range fullResults {
			if err := printResults(buf, r); err != nil {
				return nil, err
			}
		}
		ret[name] = buf.String()
	}
	return ret, nil
}

func makeJSONPath(path string) (string, error) {
	if !strings.HasPrefix(path, "$(") || !strings.HasSuffix(path, ")") {
		return "", fmt.Errorf("expected %q to start with `$(` and end with `)` for a valid JSONPath expression", path)
	}
	trimmed := strings.TrimSuffix(strings.TrimPrefix(path, "$("), ")")
	return fmt.Sprintf("{ .%s }", trimmed), nil
}

/**

NOTE; THE FOLLOWING HAS BEEN SHAMELESSLY COPIED FROM
https://github.com/tektoncd/triggers/blob/master/pkg/template/jsonpath.go

**/

func printResults(wr io.Writer, results []reflect.Value) error {
	for i, r := range results {
		text, err := textValue(r)
		if err != nil {
			return err
		}
		if i != len(results)-1 {
			text = append(text, ' ')
		}
		if _, err := wr.Write(text); err != nil {
			return err
		}
	}
	return nil
}

func textValue(v reflect.Value) ([]byte, error) {
	t := reflect.TypeOf(v.Interface())
	// special case for null values in JSON; evalToText() returns <nil> here
	if t == nil {
		return []byte("null"), nil
	}

	switch t.Kind() {
	// evalToText() returns <map> ....; return JSON string instead.
	case reflect.Map, reflect.Slice:
		return json.Marshal(v.Interface())
	default:
		return evalToText(v)
	}
}

func evalToText(v reflect.Value) ([]byte, error) {
	iface, ok := template.PrintableValue(v)
	if !ok {
		// only happens if v is a Chan or a Func
		return nil, fmt.Errorf("can't print type %s", v.Type())
	}
	var buffer bytes.Buffer
	fmt.Fprint(&buffer, iface)
	return buffer.Bytes(), nil
}
