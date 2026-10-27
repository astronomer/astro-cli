package runtimeversions

import "testing"

// pinCoversCases is the table both PinCovers implementations answer: this
// module's, for the catalog's matching, and pkg/manifest's. The root module
// holds the two to the same answers (internal/runtimecatalog), since neither
// module imports the other.
var pinCoversCases = []struct {
	Pin, Release string
	Want         bool
}{
	{"3.3", "3.3.2", true},
	{"3.3", "3.3", true},
	{"3.3.2", "3.3.2", true},
	{"3.3", "3.30.0", false},
	{"3.3", "3.4.0", false},
	{"3.3.0", "3.3.0.post1", true},
	{"3.3.0", "3.3.0+astro.1", true},
	{"3.3", "3.3.2+astro.2", true},
	{"3.3.0", "3.3.01", false},
	// A pre- or dev release only for a pin that names one.
	{"3.3", "3.3.0rc1", false},
	{"3.3.0", "3.3.0rc1", false},
	{"3.3.0", "3.3.0a1", false},
	{"3.3.0", "3.3.0.dev1", false},
	{"3.3.0rc1", "3.3.0rc1", true},
	{"3.3.0rc1", "3.3.0rc1+astro.1", true},
	{"", "3.3.0", false},
	{"3.3", "", false},
}

func TestPinCovers(t *testing.T) {
	for _, c := range pinCoversCases {
		if got := PinCovers(c.Pin, c.Release); got != c.Want {
			t.Errorf("PinCovers(%q, %q) = %v, want %v", c.Pin, c.Release, got, c.Want)
		}
	}
}
