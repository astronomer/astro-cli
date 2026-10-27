package checks_test

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// This module's dependency graph stays a near-leaf, asserted as an ALLOWLIST
// rather than a blocklist.
//
// The other promoted modules ban specific vendor chains, because they had
// specific ones to keep out. This one has none to name yet and that is the
// property worth holding: it validates DAGs by running an embedded Python
// program with the project's own interpreter, so everything expensive lives on
// the far side of a subprocess. A consumer — Astro Desktop is the first —
// inherits a version table and nothing else.
//
// An allowlist fails on the arrival of a dependency nobody considered, which is
// the case a blocklist misses. Adding an entry here is the review, so the
// question "does a DAG-parse package need this?" gets asked once.
//
// GOWORK=off, because `go list -m all` in workspace mode reports the union of
// every module in the workspace and would attribute a sibling's requires to
// this one. Stderr is captured because every way this can fail exits 1.
//
// The exec mechanics are the same as pkg/instances' and pkg/instancelocate'
// equivalents; a fix to them belongs in all three.
func TestTheModuleGraphStaysANearLeaf(t *testing.T) {
	allowed := map[string]bool{
		// This module.
		"github.com/astronomer/astro-cli/pkg/checks": true,
		// The shared table of what MWAA and Composer offer, a pure leaf.
		"github.com/astronomer/astro-cli/pkg/platformversions": true,
		// Which requirements state the Airflow version, the rule a generated
		// build and a package share (manifest.WithoutAirflow). A leaf whose one
		// dependency is the TOML parser below, which every consumer of this
		// module (the CLI, Astro Desktop) links already.
		"github.com/astronomer/astro-cli/pkg/manifest": true,
		"github.com/pelletier/go-toml/v2":              true,
		// Test-only, with the transitive set go list reports for it. Written
		// from `go list -m all` rather than from memory.
		"github.com/stretchr/testify": true,
		"github.com/stretchr/objx":    true,
		"gopkg.in/check.v1":           true,
		"gopkg.in/yaml.v3":            true,
	}

	cmd := exec.Command("go", "list", "-m", "all")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -m all: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	for _, line := range strings.Split(string(out), "\n") {
		path, _, _ := strings.Cut(strings.TrimSpace(line), " ")
		if path == "" || allowed[path] {
			continue
		}
		t.Errorf("pkg/checks requires %s, which is not on the allowlist: a package that validates DAGs "+
			"through a subprocess should need almost nothing, and a consumer inherits whatever is here. "+
			"If this dependency is right, add it above with a reason", path)
	}
}
