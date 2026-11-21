//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

// `astro bundle` is the bundle commands' home, and `astro bundle files` is
// not announced yet: the group runs and has a help page of its own, but the
// bundle commands do not list it.
//
// Tier 0: help and argument checks run before anything is read or contacted.
func TestBundleHelpListsTheCommandsAndHidesFiles(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	out := p.run("bundle", "--help").requireSuccess().output()

	for _, want := range []string{"create", "list", "update", "delete", "rollback", "versions"} {
		if !strings.Contains(out, want) {
			t.Errorf("astro bundle --help should list %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "files") {
		t.Errorf("astro bundle --help should not list the hidden files group\n%s", out)
	}

	// Hidden, yet runnable, and each command has a page of its own.
	sync := p.run("bundle", "files", "sync", "--help").requireSuccess().output()
	for _, want := range []string{"--watch", "--dry-run", "--no-delete", "--yes"} {
		if !strings.Contains(sync, want) {
			t.Errorf("astro bundle files sync --help should show %q\n%s", want, sync)
		}
	}
}

// A copy missing its destination is a usage error, as an object under json,
// before any login is asked for.
func TestBundleFilesCopyNeedsASourceAndADestination(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	var payload struct {
		Error string `json:"error"`
		Code  int    `json:"code"`
		Kind  string `json:"kind"`
	}
	p.run("bundle", "files", "cp", "my-files:a.py", "--output", "json").requireFailure().requireJSON(&payload)

	if !strings.Contains(payload.Error, "requires DST") {
		t.Errorf("the object should say what is missing, got %q", payload.Error)
	}
	if payload.Code != 2 || payload.Kind != "usage" {
		t.Errorf("want a usage error (code 2), got code %d kind %q", payload.Code, payload.Kind)
	}
}
