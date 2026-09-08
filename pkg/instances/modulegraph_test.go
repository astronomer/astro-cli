package instances_test

import (
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
	out, err := exec.Command("go", "list", "-m", "all").Output()
	if err != nil {
		t.Fatalf("go list -m all: %v", err)
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
