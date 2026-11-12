package localenv

import "testing"

// The hints are commands to run as printed. A name from a .env key can hold
// shell syntax, and stays one quoted argument; an ordinary name prints bare.
func TestHintsQuoteTheName(t *testing.T) {
	const hostile = "X; touch pwned #"
	const quoted = "'X; touch pwned #'"
	noun := Noun(KindEnv)
	for _, tc := range []struct {
		name, got, want string
	}{
		{"declare", DeclareHint(KindEnv, hostile), "astro local env " + noun + " declare " + quoted},
		{"set", SetHint(KindEnv, hostile), "astro local env " + noun + " set " + quoted},
		{"undeclare", UndeclareHint(KindEnv, hostile), "astro local env " + noun + " undeclare " + quoted},
		{"link", LinkHint(KindEnv, hostile), "astro local env " + noun + " link " + quoted},
		{"delete", removeHint(KindEnv, hostile, ScopeProject), "astro local env " + noun + " delete " + quoted + " --" + string(ScopeProject)},
		{"ordinary name", DeclareHint(KindEnv, "MY_VAR"), "astro local env " + noun + " declare MY_VAR"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}
