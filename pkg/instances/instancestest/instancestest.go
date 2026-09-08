// Package instancestest holds the test helpers pkg/instances and the two auth
// doors share, so the manifest preamble they all build links from has one
// definition.
//
// # Why it stops short of building an Instance
//
// The obvious helper would return an instances.Instance. It cannot: this
// package would then import pkg/instances, and pkg/instances' own tests are
// in-package — they call unexported functions — so importing it from there is a
// cycle Go rejects.
//
// So the split is at the manifest boundary. What lives here is everything that
// does not need pkg/instances: the preamble and its parse, the one-link
// assertion (generic over the instance type, which is what keeps it on this
// side of the cycle), a credential source's Authorization header, and an env
// lookup over a map. Each package keeps a three-line local link, whose every
// line is a call into this package — plumbing with nothing left to drift.
//
// # Why a package at all
//
// It was three identical copies, including the preamble, across three modules.
// The comment justifying that was written when the packages shared one module
// and consolidating cost one internal/ package; the split into separate modules
// turned the cheap fix into a fourth module with its own go.mod and replace
// pairs, which locked the duplication in rather than chose it. A sub-package of
// pkg/instances costs no go.mod, and both doors already require that module.
package instancestest

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"testing"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// Preamble is the project and [tool.astro] header every test manifest needs
// before its own body. Exported because a test asserting on raw TOML needs the
// same bytes the helpers parse.
var Preamble = PreambleFor("3.1")

// PreambleFor is Preamble pinning a given Airflow, for the tests that turn on
// which version a project declares — the local engine provisions different
// credentials for Airflow 2 and 3, so those tests vary the one field.
func PreambleFor(airflow string) string {
	return "[project]\n" +
		"name = 'demo'\n" +
		"requires-python = '>=3.10'\n" +
		"\n" +
		"[tool.astro]\n" +
		"airflow = '" + airflow + "'\n" +
		"workspace = 'ws_abc123'\n"
}

// Manifest parses body as a manifest, prefixed with Preamble.
//
// Through the real parser rather than a hand-built struct, so tests see links
// exactly as a user's pyproject.toml produces them — kinds and auth defaults
// included.
func Manifest(t *testing.T, body string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.Parse([]byte(Preamble + body))
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	return m
}

// OneLink asserts a built set holds exactly one instance and returns it, so a
// test that writes a one-link manifest reads as that link.
//
// Generic over the instance type, and taking the names for its message rather
// than the set itself, because naming instances.Set here would import
// pkg/instances and cost the cycle this package exists on the far side of.
func OneLink[T any](t *testing.T, all []T, names []string) T {
	t.Helper()
	if len(all) != 1 {
		t.Fatalf("expected one link, got %v", names)
	}
	return all[0]
}

// Header runs a credential source and returns the Authorization header it would
// produce, which is what actually matters about one. Empty for a source that
// sends no credential, which is a real answer.
func Header(t *testing.T, src airflowapi.CredentialSource) string {
	t.Helper()
	h, err := header(src)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return h
}

// header is Header's answer without a testing.T, so this package's own tests
// can assert on the refusal below rather than being killed by it.
func header(src airflowapi.CredentialSource) (string, error) {
	if src == nil {
		return "", nil
	}
	scheme, value, err := src(context.Background())
	if err != nil {
		return "", fmt.Errorf("credentials: %w", err)
	}
	if scheme == "" {
		// A value with no scheme would be dropped here and read as "sends no
		// credential", so a source leaking one past a test asserting silence
		// would pass. Refuse instead of conflating the two.
		if value != "" {
			return "", errors.New("credential source returned a value with no scheme, which no caller can send")
		}
		return "", nil
	}
	return scheme + " " + value, nil
}

// Env builds a LookupEnv over a fixed map, so no test touches the process
// environment.
//
// Cloned, so a caller that keeps mutating the map it passed cannot change what
// the environment reports mid-test.
func Env(pairs map[string]string) func(string) (string, bool) {
	pairs = maps.Clone(pairs)
	return func(name string) (string, bool) {
		v, ok := pairs[name]
		return v, ok
	}
}
