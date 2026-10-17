package instances

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/instances/instancestest"
)

const apcLinkBody = `
[tool.astro.deployments.prod-apc]
target = 'apc'
domain = 'astro.corp.example'
workspace = 'ckws000000000000000000001'
deployment = 'ckdep00000000000000000001'
default = true
`

// An apc link is in the set like any other, listed with where it points, and
// it is what the default resolves to when it is the default.
func TestAnAPCLinkIsListedAndResolved(t *testing.T) {
	set := Build(instancestest.Manifest(t, apcLinkBody))
	it, ok := set.Lookup("prod-apc")
	if !ok {
		t.Fatalf("prod-apc missing from the set: %v", set.Names())
	}
	if it.Kind != KindAPC || it.Where != "deployment ckdep00000000000000000001 on astro.corp.example" {
		t.Errorf("prod-apc = {%s %q}, want {apc \"deployment ... on astro.corp.example\"}", it.Kind, it.Where)
	}
	sel, err := set.Select(Request{})
	if err != nil {
		t.Fatal(err)
	}
	if sel.Instance.Name != "prod-apc" {
		t.Errorf("the default resolved to %q, want prod-apc", sel.Instance.Name)
	}
}

// -d with the id an apc link carries resolves to that link, so it is refused
// as the link named is, never read as an Astro Deployment id.
func TestAnAPCDeploymentIDResolvesToItsLink(t *testing.T) {
	set := Build(instancestest.Manifest(t, apcLinkBody))
	sel, err := set.Select(Request{Flag: "ckdep00000000000000000001"})
	if err != nil {
		t.Fatal(err)
	}
	if sel.Instance.Name != "prod-apc" || sel.Instance.Kind != KindAPC {
		t.Fatalf("selected %q (%s), want the prod-apc link", sel.Instance.Name, sel.Instance.Kind)
	}
	_, err = sel.Instance.Transport(context.Background(), Deps{})
	if !errors.Is(err, ErrAPCUnsupported) {
		t.Errorf("Transport: err = %v, want ErrAPCUnsupported", err)
	}
	// An id no link carries is still an Astro Deployment.
	if it := set.DeploymentByID("clm2xk9dq000108l7a2b3c4d5"); it.Kind != KindAstro {
		t.Errorf("an unlinked id = %s, want astro", it.Kind)
	}
}

// Reaching an apc link's Airflow is not supported yet, so both doors refuse it
// by name before any lookup: the Locator would otherwise be asked to find an
// Astro Deployment by a Houston id.
func TestAnAPCLinkIsRefusedBeforeAnyLookup(t *testing.T) {
	it, _ := Build(instancestest.Manifest(t, apcLinkBody)).Lookup("prod-apc")
	d := Deps{Locator: locatorFunc(func(context.Context, Instance) (string, error) {
		t.Fatal("an apc link reached the Locator")
		return "", nil
	})}
	_, err := it.Transport(context.Background(), d)
	if !errors.Is(err, ErrAPCUnsupported) || !strings.Contains(err.Error(), `"prod-apc"`) {
		t.Errorf("Transport: err = %v, want ErrAPCUnsupported naming prod-apc", err)
	}
	_, err = it.HTTPDoorFor(context.Background(), d)
	if !errors.Is(err, ErrAPCUnsupported) {
		t.Errorf("HTTPDoorFor: err = %v, want ErrAPCUnsupported", err)
	}
}
