package config

import (
	"bytes"
	"os"
	"testing"
	"time"
)

// A selection made under a token on a host with no saved context is not saved
// (it would be a partial entry for a host nobody logged in to), and says so; on
// a host with a saved context it is stored as ever.
func TestASelectionOnAContextThatExistsOnlyForATokenIsNotSaved(t *testing.T) {
	loginHome(t, plaintextConfig+`  other_io:
    domain: other.io
    token: Bearer other-access
`, nil)
	t.Setenv("ASTRO_DOMAIN", "")
	var notes []string
	prev := unsavedSelectionNote
	unsavedSelectionNote = func(cKey, field string) { notes = append(notes, cKey+":"+field) }
	t.Cleanup(func() { unsavedSelectionNote = prev })
	before, err := os.ReadFile(HomeConfigFile)
	if err != nil {
		t.Fatal(err)
	}

	held := Context{Domain: "held.io"}
	if err := held.UseEnvironmentLogin(envToken, time.Time{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(forgetEnvironmentLogin)
	for _, field := range []string{"workspace", "last_used_workspace", "organization"} {
		if err := held.SetContextKey(field, "chosen"); err != nil {
			t.Fatal(err)
		}
	}
	if len(notes) != 3 {
		t.Errorf("notes = %v, want one per selection", notes)
	}
	// What an organization switch writes back as it read it is no selection:
	// not saved either, and no note.
	for _, field := range []string{"token", "refreshtoken", "user_email"} {
		if err := held.SetContextKey(field, "as-read"); err != nil {
			t.Fatal(err)
		}
	}
	if len(notes) != 3 {
		t.Errorf("a login field written back gave a note: %v", notes)
	}
	after, err := os.ReadFile(HomeConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Errorf("the config changed:\n%s", after)
	}
	if viperHome.IsSet(contextsKey + ".held_io") {
		t.Error("a partial entry exists in memory for the token's host")
	}

	// A host with a saved context keeps working as before.
	other := Context{Domain: "other.io"}
	if err := other.SetContextKey("workspace", "chosen"); err != nil {
		t.Fatal(err)
	}
	if got, err := other.GetContext(); err != nil || got.Workspace != "chosen" {
		t.Errorf("a saved context's selection was not stored: %q (%v)", got.Workspace, err)
	}
	if len(notes) != 3 {
		t.Errorf("a saved context's selection gave a note: %v", notes)
	}
}
