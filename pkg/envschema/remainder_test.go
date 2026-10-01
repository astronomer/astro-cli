package envschema

import "testing"

func TestRemainderAfterDelete(t *testing.T) {
	required := &ValueSpec{}
	optional := &ValueSpec{Optional: true}
	cases := []struct {
		name     string
		spec     *ValueSpec
		supplied bool
		want     Remainder
	}{
		{"undeclared", nil, false, RemainderUndeclared},
		{"undeclared and supplied", nil, true, RemainderUndeclared},
		{"required, nothing left", required, false, RemainderRequired},
		{"optional, nothing left", optional, false, RemainderAbsent},
		{"required, supplied", required, true, RemainderSupplied},
		{"optional, supplied", optional, true, RemainderSupplied},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RemainderAfterDelete(tc.spec, tc.supplied); got != tc.want {
				t.Errorf("RemainderAfterDelete = %q, want %q", got, tc.want)
			}
		})
	}
}
