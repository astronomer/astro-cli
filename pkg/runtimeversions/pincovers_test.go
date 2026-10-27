package runtimeversions

import "testing"

// The catalog's rule for which releases an Airflow pin covers agrees with
// manifest.PinCovers, PEP 440's apache-airflow==<pin>.*, which this module
// cannot import: a prefix match on release segments that covers pre-, post-
// and dev releases of the pinned release and compares no local label.
func TestPinCoversMatchesPEP440PrefixMatching(t *testing.T) {
	cases := []struct {
		pin, release string
		want         bool
	}{
		{"3.3", "3.3.2", true},
		{"3.3", "3.3", true},
		{"3.3.2", "3.3.2", true},
		{"3.3", "3.30.0", false},
		{"3.3", "3.4.0", false},
		{"3.3.0", "3.3.0rc1", true},
		{"3.3.0", "3.3.0a1", true},
		{"3.3.0", "3.3.0.post1", true},
		{"3.3.0", "3.3.0.dev1", true},
		{"3.3", "3.3.0rc1", true},
		{"3.3.0", "3.3.0+astro.1", true},
		{"3.3.0", "3.3.01", false},
		{"", "3.3.0", false},
	}
	for _, c := range cases {
		if got := pinCovers(c.pin, c.release); got != c.want {
			t.Errorf("pinCovers(%q, %q) = %v, want %v", c.pin, c.release, got, c.want)
		}
	}
}
