package instances_test

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The core's MODULE graph carries neither vendor chain, not just its import
// graph.
//
// These are two different properties and only one of them was held. archlint's
// TestTheAuthDoorsStayOptional asks `go list -deps`, which is the import graph:
// add `github.com/aws/aws-sdk-go-v2` to this module's go.mod without importing
// it from any file — or let pkg/manifest grow such a require — and that test
// stays green while this property is gone.
//
// The module half is the one a consumer feels. It is what keeps the AWS SDK out
// of Astro Desktop's go.sum, out of its dependency graph, and therefore out of
// its Dependabot alert surface. Asserted here rather than in archlint because
// archlint lives in the root module and `go list -m all` answers per module.
func TestTheModuleGraphCarriesNoVendorAuthChain(t *testing.T) {
	// GOWORK=off, because `go list -m all` in workspace mode reports the union
	// of every module in the workspace: with a go.work covering the sibling
	// sub-modules this reads pkg/awsauth's requires as this module's own and
	// fails naming the exact regression it exists to prevent. A go.work at the
	// repo root is an ordinary thing to have in a repo of twenty sub-modules,
	// and it is not gitignored.
	//
	// Stderr is captured, not discarded. An unresolvable require, a missing
	// go.sum entry and a proxy outage all exit 1, and `%v` on the ExitError
	// alone renders every one of them as "exit status 1" — which cost a
	// debugging cycle when this test was first mutated.
	cmd := exec.Command("go", "list", "-m", "all")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -m all: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	forbidden := []string{
		"github.com/aws/",
		"golang.org/x/oauth2",
		"cloud.google.com/",
		"google.golang.org/api",
	}
	for _, line := range strings.Split(string(out), "\n") {
		path, _, _ := strings.Cut(strings.TrimSpace(line), " ")
		for _, prefix := range forbidden {
			if strings.HasPrefix(path, prefix) {
				t.Errorf("pkg/instances requires %s: a consumer of the core would carry it in go.sum "+
					"and in its dependency graph, which is what pkg/awsauth and pkg/googleauth exist to avoid", path)
			}
		}
	}
}
