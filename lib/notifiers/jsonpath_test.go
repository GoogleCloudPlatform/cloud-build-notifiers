package notifiers

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
)

func TestNewResolver(t *testing.T) {
	substs := map[string]string{
		"_FOO": "$(thing.other-thing.foo)",
		"_BAR": "$(pizzas[4].cheese)",
	}

	cfg := &Config{
		Spec: &Spec{
			Notification: &Notification{
				Substitutions: substs,
			},
		},
	}

	if _, err := newResolver(cfg); err != nil {
		t.Fatalf("newResolver(%v) failed unexpectedly: %v", cfg, err)
	}
}

func TestNewResolverErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		substs map[string]string
	}{{
		name: "no underscore prefix subst name",
		substs: map[string]string{
			"PIZZA": "$(foo.bar[2])",
		},
	}, {
		name: "bad JSONPath",
		substs: map[string]string{
			"_PIZZA": "$(])",
		},
	}, {
		name: "no enclosing $()",
		substs: map[string]string{
			"_PIZZA": "hello.goodbye",
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				Spec: &Spec{
					Notification: &Notification{
						Substitutions: tc.substs,
					},
				},
			}

			if _, err := newResolver(cfg); err == nil {
				t.Errorf("newResolver(%v) unexpectedly succeeded", cfg)
			} else {
				t.Logf("got expected error %v", err)
			}
		})
	}
}

type fakeSecretGetter struct {
	secrets map[string]string
}

func (f *fakeSecretGetter) GetSecret(_ context.Context, secretResource string) (string, error) {
	s, ok := f.secrets[secretResource]
	if !ok {
		return "", fmt.Errorf("secret not stored: %q", secretResource)
	}
	return s, nil
}

func TestResolve(t *testing.T) {
	sg := &fakeSecretGetter{
		secrets: map[string]string{
			"projects/some-project/secrets/some-password/versions/latest": "top-secret",
		},
	}

	secrets := []*Secret{
		{
			LocalName:    "some-password",
			ResourceName: "projects/some-project/secrets/some-password/versions/latest",
		},
	}

	substs := map[string]string{
		"_BUILD_STATUS":        "$(build.status)",
		"_BRANCH_NAME":         "$(build.substitutions.BRANCH_NAME)",
		"_COMMIT_AUTHOR_EMAIL": "$(build.substitutions._COMMIT_AUTHOR_EMAIL)",
		"_SOME_PASSWORD":       "$(secrets.some-password)",
		"_ALL_STEPS":           "$(build.steps[*].name)",
		"_MY_TRIGGER_ID":       "$(build.build_trigger_id)",
	}

	cfg := &Config{
		Spec: &Spec{
			Notification: &Notification{
				Substitutions: substs,
			},
			Secrets: secrets,
		},
	}

	r, err := newResolver(cfg)
	if err != nil {
		t.Fatal(err)
	}

	build := &cbpb.Build{
		Status: cbpb.Build_SUCCESS,
		Steps: []*cbpb.BuildStep{
			{Name: "foo"},
			{Name: "bar"},
			{Name: "baz"},
		},
		Substitutions: map[string]string{
			"BRANCH_NAME":          "my-branch",
			"_COMMIT_AUTHOR_EMAIL": "me@example.com",
		},
	}

	gotResolved, err := r.Resolve(context.Background(), sg, build)
	if err != nil {
		t.Fatal(err)
	}

	wantResolved := map[string]string{
		"$_BRANCH_NAME":         "my-branch",
		"$_BUILD_STATUS":        "SUCCESS",
		"$_COMMIT_AUTHOR_EMAIL": "me@example.com",
		"$_SOME_PASSWORD":       "top-secret",
		"$_ALL_STEPS":           "foo bar baz",
		"$_MY_TRIGGER_ID":       "",
	}

	if diff := cmp.Diff(wantResolved, gotResolved); diff != "" {
		t.Errorf("unxpected diff from resolving JSONPath:\n%s", diff)
	}
}

func TestResolveErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		substs  map[string]string
		secrets []*Secret
		build   *cbpb.Build
	}{{
		name: "path for list with bad index",
		substs: map[string]string{
			"_FOO": "$(build.tags[404])",
		},
		build: &cbpb.Build{Id: "id", Tags: []string{"some-tag"}},
	}, {
		name: "path for unknown subst",
		substs: map[string]string{
			"_FOO": "$(build.substitutions['DNE'])",
		},
		build: &cbpb.Build{Substitutions: map[string]string{"_HELLO": "world"}},
	}, {
		name: "path for unknown field",
		substs: map[string]string{
			"_FOO": "$(build.banana)",
		},
		build: &cbpb.Build{Id: "id"},
	}, {
		name: "path for unknown secret",
		substs: map[string]string{
			"_FOO": "$(secrets['not-found'])",
		},
	}, {
		name: "path for unfetchable secret",
		substs: map[string]string{
			"_FOO": "$(secrets['bad-secret'])",
		},
		secrets: []*Secret{
			{
				LocalName:    "bad-secret",
				ResourceName: "projects/foo/secrets/bad-secret/version/latest",
			},
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				Spec: &Spec{
					Notification: &Notification{
						Substitutions: tc.substs,
					},
					Secrets: tc.secrets,
				},
			}
			r, err := newResolver(cfg)
			if err != nil {
				t.Fatalf("newResolver(...) failed unexpectedly: %v", err)
			}

			// Any secrets we try to look up will result in errors.
			sg := new(fakeSecretGetter)

			if _, err := r.Resolve(context.Background(), sg, tc.build); err == nil {
				t.Error("Resolve unexpectedly succeeded")
			} else {
				t.Logf("got expected error: %v", err)
			}
		})
	}
}
