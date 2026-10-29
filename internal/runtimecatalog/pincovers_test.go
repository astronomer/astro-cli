package runtimecatalog

import (
	"testing"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// The rule for which Airflow releases a pin covers is spelled twice, once in
// pkg/manifest and once in pkg/runtimeversions, which import nothing of each
// other. The catalog picks builds by one and the manifest's stale-pin and
// locked-build checks by the other, so they must agree; this module imports
// both and holds them to the same answers.
func TestBothPinCoversAgree(t *testing.T) {
	for _, c := range []struct {
		pin, release string
		want         bool
	}{
		{"3.3", "3.3.2", true},
		{"3.3", "3.3", true},
		{"3.3", "3.30.0", false},
		{"3.3", "3.4.0", false},
		{"3.3.0", "3.3.0.post1", true},
		{"3.3", "3.3.2+astro.2", true},
		{"3.3", "3.3.0rc1", false},
		{"3.3.0", "3.3.0.dev1", false},
		{"3.3.0rc1", "3.3.0rc1+astro.1", true},
		{"", "3.3.0", false},
	} {
		m, r := manifest.PinCovers(c.pin, c.release), runtimeversions.PinCovers(c.pin, c.release)
		if m != c.want || r != c.want {
			t.Errorf("PinCovers(%q, %q): manifest %v, runtimeversions %v, want %v", c.pin, c.release, m, r, c.want)
		}
	}
}
