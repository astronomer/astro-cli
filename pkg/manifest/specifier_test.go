package manifest

import "testing"

func TestRequirementAdmits(t *testing.T) {
	for _, tc := range []struct {
		req, version  string
		admits, known bool
	}{
		{"pandas", "2.3.3", true, true},
		{"pandas>=2.3", "2.3.3", true, true},
		{"pandas>=2.3.3", "2.3.3", true, true},
		{"Pandas >= 3", "2.3.3", false, true},
		{"pandas>=2,<3", "2.3.3", true, true},
		{"pandas[performance]>=2", "2.3.3", true, true},
		{"pandas (>=2)", "2.3.3", true, true},
		{"pandas==2.3.*", "2.3.3", true, true},
		{"pandas==2.2.*", "2.3.3", false, true},
		{"pandas!=2.3.3", "2.3.3", false, true},
		{"pandas~=2.2", "2.3.3", true, true},
		{"pandas~=2.2.0", "2.3.3", false, true},
		{"numpy<2", "1.26.4", true, true},
		{"numpy<2", "2.0.0rc1", false, true},
		{"numpy<2.0rc2", "2.0.0rc1", true, true},
		{"x>1.0", "1.0.post1", false, true},
		{"x>=1.0", "1.0.post1", true, true},
		{"x>=0.60b0", "0.63b1", true, true},
		{"x<1.0", "1.0.dev1", false, true},
		{"x==1.0", "1.0.0", true, true},
		{"x<=2026.7", "2026.7.22", false, true},
		{"x>=1", "1.2.3+astro.1", true, true},
		{"x>=3; python_version < '3.11'", "2.0", false, false},
		{"x @ https://h/x.whl", "1.0", false, false},
		{"x===1.0", "1.0", false, false},
		{"x>=1", "not a version", false, false},
	} {
		admits, known := RequirementAdmits(tc.req, tc.version)
		if admits != tc.admits || known != tc.known {
			t.Errorf("RequirementAdmits(%q, %q) = %v, %v; want %v, %v", tc.req, tc.version, admits, known, tc.admits, tc.known)
		}
	}
}
