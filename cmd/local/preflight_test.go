package local

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/checks"
)

func TestProvisionerKeyIsStableAndSpecific(t *testing.T) {
	p := &uvProvisioner{cacheDir: "/cache"}
	base := checks.VenvSpec{Airflow: "3.0.6", Python: "3.12", Reqs: []string{"apache-airflow==3.0.6", "pandas"}}

	k1 := p.key(base)
	k2 := p.key(base)
	if k1 != k2 {
		t.Errorf("same spec must hash to the same key: %q vs %q", k1, k2)
	}
	if !strings.HasPrefix(k1, "af3.0.6-py3.12-") {
		t.Errorf("key should carry a readable version/python prefix: %q", k1)
	}

	// A changed dependency, Airflow, or Python must land in a different key.
	changed := []checks.VenvSpec{
		{Airflow: "3.0.6", Python: "3.12", Reqs: []string{"apache-airflow==3.0.6", "numpy"}},
		{Airflow: "3.1.8", Python: "3.12", Reqs: []string{"apache-airflow==3.0.6", "pandas"}},
		{Airflow: "3.0.6", Python: "3.11", Reqs: []string{"apache-airflow==3.0.6", "pandas"}},
	}
	for _, spec := range changed {
		if p.key(spec) == k1 {
			t.Errorf("a changed spec must change the key: %+v", spec)
		}
	}
}

func TestResolveConstraintsOfflineDegrades(t *testing.T) {
	// A fetch failure returns ErrConstraintsUnavailable without touching uv, so
	// the offline path never needs a binary.
	p := &uvProvisioner{
		fetch: func(context.Context, string) ([]byte, error) {
			return nil, errors.New("dial tcp: no route to host")
		},
	}
	err := p.ResolveConstraints(context.Background(), []string{"pandas"}, "https://example/constraints.txt", "3.12")
	if !errors.Is(err, checks.ErrConstraintsUnavailable) {
		t.Errorf("an unreachable constraints file should degrade to ErrConstraintsUnavailable, got %v", err)
	}
}
