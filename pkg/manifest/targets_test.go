package manifest

import (
	"reflect"
	"strings"
	"testing"
)

const targetsHead = "[project]\nname = \"p\"\n\n[tool.astro]\nairflow = \"3.1\"\n"

func warningKeys(m *Manifest) []string {
	var keys []string
	for _, w := range m.Warnings {
		keys = append(keys, w.Key)
	}
	return keys
}

// The keys the readers use load clean, and so do the reserved sections,
// whatever they carry: nothing reads them, so there is nothing to misspell.
func TestTargetSectionsWithKnownKeysLoadWithoutWarnings(t *testing.T) {
	m, err := Parse([]byte(targetsHead + `
[tool.astro.targets.mwaa]
region = "us-east-1"
bucket = "s3://acme-airflow-orders"

[tool.astro.targets.composer]
project = "acme-data"
location = "us-central1"

[tool.astro.targets.astro]
image = { os = "ubi", python = "3.12" }

[tool.astro.targets.oss]
anything = "goes"
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Warnings) != 0 {
		t.Errorf("want no warnings, got %+v", m.Warnings)
	}
}

// A misspelled key in a section something reads is a warning, not a refusal:
// the manifest still loads, the section is still carried whole, and the
// warning names the key and the ones the section takes.
func TestUnknownTargetKeysWarnWithoutFailingTheLoad(t *testing.T) {
	m, err := Parse([]byte(targetsHead + `
[tool.astro.targets.mwaa]
regoin = "us-east-1"

[tool.astro.targets.composer]
project = "acme-data"
zone = "us-central1-a"

[tool.astro.targets.mwa]
region = "us-east-1"
`))
	if err != nil {
		t.Fatalf("unknown target keys must not fail the load: %v", err)
	}
	want := []string{
		"tool.astro.targets.composer.zone",
		"tool.astro.targets.mwa",
		"tool.astro.targets.mwaa.regoin",
	}
	if got := warningKeys(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("warning keys = %q, want %q", got, want)
	}
	for _, w := range m.Warnings {
		if w.Code != CodeUnknownKey {
			t.Errorf("%s: code = %q, want %q", w.Key, w.Code, CodeUnknownKey)
		}
	}
	if r := m.Warnings[2].Reason; !strings.Contains(r, "bucket, region") {
		t.Errorf("reason %q should name the keys the mwaa section takes", r)
	}
	if m.Astro.Targets["mwaa"]["regoin"] != "us-east-1" {
		t.Errorf("the section should still be carried whole, got %#v", m.Astro.Targets["mwaa"])
	}
}

// Warnings come back in one order, though target sections decode out of a map.
func TestTargetWarningOrderIsStable(t *testing.T) {
	content := targetsHead + "\n[tool.astro.targets.mwaa]\n"
	for _, k := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		content += k + " = \"x\"\n"
	}
	var first []string
	for range 100 {
		m, err := Parse([]byte(content))
		if err != nil {
			t.Fatal(err)
		}
		got := warningKeys(m)
		if first == nil {
			first = got
			continue
		}
		if !reflect.DeepEqual(got, first) {
			t.Fatalf("order changed between parses: %q then %q", first, got)
		}
	}
}

// A manifest that fails validation reports its problems, not its warnings:
// the ValidationError is the whole answer until it loads.
func TestWarningsDoNotBecomeProblems(t *testing.T) {
	_, err := Parse([]byte("[project]\nname = \"p\"\n\n[tool.astro]\nairflow = \"x\"\n\n[tool.astro.targets.mwaa]\nregoin = \"us-east-1\"\n"))
	ve := validationError(t, err)
	if got := problemKeys(ve); !reflect.DeepEqual(got, []string{"tool.astro.airflow"}) {
		t.Errorf("problem keys = %q, want only the airflow pin", got)
	}
}
