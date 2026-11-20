package localenv

import "testing"

// Output names each store for what it is, the three vaults alike, in text and
// json, and drops a workspace's id but keeps why it could not be read. A name
// passes through, so mapping an already-named row again changes nothing.
func TestSourceName(t *testing.T) {
	for source, want := range map[string]string{
		SourceProject:                         ".env",
		SourceVault:                           "project vault",
		SourceGlobalVault:                     "global vault",
		SourceShell:                           "shell",
		"default":                             "default",
		SourceAbsent:                          "absent",
		"workspace":                           "cloud vault",
		WorkspaceSource("cmws"):               "cloud vault",
		"workspace (unavailable: no route)":   "cloud vault (unavailable: no route)",
		".env":                                ".env",
		"global vault":                        "global vault",
		"cloud vault (unavailable: no route)": "cloud vault (unavailable: no route)",
	} {
		if got := SourceName(source); got != want {
			t.Errorf("SourceName(%q) = %q, want %q", source, got, want)
		}
	}
}
