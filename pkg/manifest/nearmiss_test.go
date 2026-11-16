package manifest

import (
	"errors"
	"testing"
)

func TestNearMiss(t *testing.T) {
	for name, want := range map[string]string{
		"defalt":           "default",
		"defaults":         "default",
		"Default":          "default",
		"workspce":         "workspace",
		"build_secrets":    "build-secrets",
		"pakages":          "packages",
		"deployment-id":    "",
		"something-new":    "",
		"orgnization":      "organization",
		"include-deferred": "include_deferred",
	} {
		known := append(append(append([]string{}, astroKeys...), linkKeys...), poolKeys...)
		if got := nearMiss(name, known); got != want {
			t.Errorf("nearMiss(%q) = %q, want %q", name, got, want)
		}
	}
}

// A near-miss of a known key is refused, naming the key it was meant to be;
// any other unknown key loads with a warning.
func TestANearMissIsRefusedAndANewKeyWarns(t *testing.T) {
	const base = "[project]\nname = 'p'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\nworkspace = 'ws'\n"
	_, err := Parse([]byte(base + "\n[tool.astro.deployments.prod]\ndeployment = 'dep'\ndefalt = true\n"))
	var ve *ValidationError
	if !errors.As(err, &ve) || len(ve.Problems) != 1 || ve.Problems[0].Key != "tool.astro.deployments.prod.defalt" ||
		ve.Problems[0].Reason != "not a key: did you mean default?" {
		t.Errorf("want the near-miss refused, got %v", err)
	}

	m, err := Parse([]byte(base + "brand-new-key = 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Warnings) != 1 || m.Warnings[0].Key != "tool.astro.brand-new-key" {
		t.Errorf("want one warning, got %+v", m.Warnings)
	}
}

// What keeps an unknown key a warning safe for a newer reader's keys: no key
// a table defines is a near-miss of another in the same table, so an older
// reader never refuses a newer key as a typo of one it knows. A key added too
// close to an existing one fails here, while it is being added, rather than
// in an older CLI or Astro Desktop in front of a user. target and targets
// predate the rule; an older reader knows both, so neither is new to it.
func TestNoKeyIsANearMissOfAnother(t *testing.T) {
	allowed := map[[2]string]bool{{"target", "targets"}: true, {"targets", "target"}: true}
	for table, keys := range map[string][]string{
		"[tool.astro]":        astroKeys,
		"a link":              linkKeys,
		"a pool":              poolKeys,
		"[tool.astro.deploy]": deployKeys,
	} {
		for i, a := range keys {
			others := append(append([]string{}, keys[:i]...), keys[i+1:]...)
			if b := nearMiss(a, others); b != "" && !allowed[[2]string{a, b}] {
				t.Errorf("in %s, %s is a near-miss of %s: an older reader would refuse one as a typo of the other", table, a, b)
			}
		}
	}
}

// Load hands a manifest's warnings to the handler, with the file's path.
func TestLoadHandsWarningsToTheHandler(t *testing.T) {
	var gotPath string
	var got []Problem
	SetWarningHandler(func(path string, w []Problem) { gotPath, got = path, w })
	t.Cleanup(func() { SetWarningHandler(nil) })

	path := write(t, "[project]\nname = 'p'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\nbrand-new-key = 1\n")
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
	if gotPath != path || len(got) != 1 || got[0].Key != "tool.astro.brand-new-key" {
		t.Errorf("got %q %+v", gotPath, got)
	}
}
