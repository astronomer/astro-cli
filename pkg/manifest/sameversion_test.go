package manifest

import "testing"

func TestSameVersionComparesAsPEP440Orders(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.0.0.post0", "1.0.0-post0", true},
		{"2.0", "2.0.0", true},
		{"2.3.3", "2.3.3", true},
		{"2.3.3", "2.3.4", false},
		{"3.3.2+astro.1", "3.3.2+astro.2", false},
		{"not a version", "not a version", true},
		{"not a version", "1.0", false},
	}
	for _, c := range cases {
		if got := SameVersion(c.a, c.b); got != c.want {
			t.Errorf("SameVersion(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
