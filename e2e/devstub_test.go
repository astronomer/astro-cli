//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The whole v1 `astro dev` tree is one stub in v2: any subcommand fails, names
// its v2 replacement, and says the old one is gone — so a script or a CI job
// breaks loudly instead of quietly doing nothing.
//
// The mapping is written out here rather than imported from pkg/scaffold, which
// is where the CLI reads it. That is the point: this is the published contract,
// and a test that asks the implementation what it promises cannot notice a
// promise being broken. If a row here and a row there disagree, one of them is
// a decision someone should be making on purpose.
var devMapping = []struct {
	typed       string
	replacement string
}{
	{"start", "astro local start"},
	{"stop", "astro local stop"},
	{"restart", "astro local restart"},
	{"ps", "astro local status"},
	{"logs", "astro local logs"},
	{"run", "astro local run"},
	{"bash", "astro local shell"},
	{"parse", "astro local check"},
	{"build", "astro package"},
	{"kill", "astro local reset --yes"},
	{"pytest", "uv run pytest"},
	{"init", "astro init"},
	// v1 bulk-loaded connections and variables into a running Airflow. v2
	// declares them per name, so these name the command that does the job
	// rather than a bulk equivalent that does not exist.
	//
	// Longest prefix first, and these three are what makes that matter: if
	// `object` were matched before `object import`, the guidance for the most
	// specific thing someone typed would be the vaguest one available. That
	// property needs no case of its own — TestDevSubcommandsAreRemoved runs
	// every row below and asserts the replacement each one maps to.
	//
	// `object export` is the row that carries that check now: there is no
	// `object import` row any more, because the only honest answer for it was
	// the same tree the `object` row names and a duplicate row publishes a
	// duplicate line in the stub's payload. So a shorter prefix winning
	// shows up as `object export` answering with "astro local env".
	{"object export", "astro local env list"},
	{"object", "astro local env"},
}

func TestDevSubcommandsAreRemoved(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	for _, tc := range devMapping {
		t.Run(tc.typed, func(t *testing.T) {
			r := p.run(append([]string{"dev"}, strings.Fields(tc.typed)...)...).requireFailure()
			r.requireStderr("removed in Astro CLI v2")
			r.requireStderr("Use " + tc.replacement + " instead")
		})
	}
}

// The count is part of the contract too: a row quietly dropped from the mapping
// would leave every remaining case passing.
func TestDevMappingCoversEveryCommand(t *testing.T) {
	tier(t, 0)

	var payload struct {
		Mapping []struct {
			Command     string `json:"command"`
			Replacement string `json:"replacement"`
		} `json:"mapping"`
	}
	newProject(t).run("dev", "ps", "--output", "json").requireFailure().requireJSON(&payload)

	if len(payload.Mapping) != len(devMapping) {
		t.Errorf("the CLI publishes %d mappings, this suite pins %d", len(payload.Mapping), len(devMapping))
	}
	published := map[string]string{}
	for _, m := range payload.Mapping {
		published[m.Command] = m.Replacement
	}
	for _, want := range devMapping {
		if got, ok := published[want.typed]; !ok {
			t.Errorf("astro dev %s is missing from the published mapping", want.typed)
		} else if got != want.replacement {
			t.Errorf("astro dev %s maps to %q, want %q", want.typed, got, want.replacement)
		}
	}
}

// Bare `astro dev`, which is the one case with no subcommand to name.
func TestDevBareIsRemoved(t *testing.T) {
	tier(t, 0)

	newProject(t).run("dev").
		requireFailure().
		requireStderr("astro dev was removed in Astro CLI v2").
		requireStderr("Local Airflow now lives under astro local")
}

// A subcommand v1 never had says so, rather than guessing a replacement.
func TestDevUnknownSubcommandHasNoReplacement(t *testing.T) {
	tier(t, 0)

	r := newProject(t).run("dev", "nonsense").requireFailure()
	r.requireStderr("has no direct replacement")
	if strings.Contains(r.Stderr, "Use `") {
		t.Errorf("an unknown subcommand was given a replacement anyway\n%s", r.output())
	}
}

// Old invocations carry v1 flags this stub never declared. Parsing them would
// fail before the guidance printed, which is the whole reason the command
// disables flag parsing — so the flags have to survive a real invocation.
func TestDevKeepsItsGuidanceWhenOldFlagsArePassed(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	for _, args := range [][]string{
		{"dev", "start", "--no-cache"},
		{"dev", "logs", "--follow", "--scheduler"},
		{"dev", "restart", "-n", "something"},
	} {
		t.Run(strings.Join(args[1:], " "), func(t *testing.T) {
			p.run(args...).requireFailure().requireStderr("removed in Astro CLI v2")
		})
	}
}

// A flag's value must not be echoed back as the command someone typed.
func TestDevDoesNotEchoAFlagValueAsTheCommand(t *testing.T) {
	tier(t, 0)

	var payload struct {
		Typed string `json:"typed_command"`
	}
	newProject(t).run("dev", "logs", "--since", "scheduler", "--output", "json").
		requireFailure().
		requireJSON(&payload)

	if payload.Typed != "astro dev logs" {
		t.Errorf("typed_command = %q, want %q", payload.Typed, "astro dev logs")
	}
}

// The JSON payload is what a script or an agent reads, so its keys are pinned
// here and the guidance stays off stdout.
func TestDevJSONPayload(t *testing.T) {
	tier(t, 0)

	r := newProject(t).run("dev", "ps", "--output", "json").requireFailure()

	var payload map[string]any
	r.requireJSON(&payload)

	for key, want := range map[string]string{
		"typed_command": "astro dev ps",
		"replacement":   "astro local status",
	} {
		if got, _ := payload[key].(string); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if payload["error"] == nil {
		t.Errorf("the payload names no error\n%s", r.output())
	}
	if mapping, _ := payload["mapping"].([]any); len(mapping) == 0 {
		t.Errorf("the payload carries no mapping\n%s", r.output())
	}
	// The payload carries no "doc" key. Its absence is part of the contract,
	// so a key appearing here is a change to what consumers parse rather than
	// an addition they can ignore.
	if _, ok := payload["doc"]; ok {
		t.Error("the payload grew a `doc` key")
	}
	// The human guidance is prose and belongs on stderr. On stdout it would sit
	// in front of the payload a consumer is parsing.
	if strings.Contains(r.Stdout, "Local Airflow now lives under") {
		t.Errorf("the human guidance reached stdout in json mode\n%s", r.output())
	}
}

// Both spellings of the flag, since the stub matches them by hand rather than
// through cobra.
func TestDevJSONFlagSpellings(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	for _, args := range [][]string{
		{"dev", "ps", "--output", "json"},
		{"dev", "ps", "--output=json"},
		{"dev", "ps", "-o", "json"},
		{"dev", "ps", "-o=json"},
	} {
		t.Run(strings.Join(args[2:], " "), func(t *testing.T) {
			var payload struct {
				Replacement string `json:"replacement"`
			}
			p.run(args...).requireFailure().requireJSON(&payload)
			if payload.Replacement != "astro local status" {
				t.Errorf("replacement = %q, want %q", payload.Replacement, "astro local status")
			}
		})
	}
}

// A classic 1.x project that also keeps a pyproject.toml for tool settings is
// still 1.x, so the stub leads with the conversion: `uv run pytest` does not
// work until `astro init` has made the directory a project.
func TestDevInA1xProjectLeadsWithTheConversion(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	if err := os.Mkdir(filepath.Join(p.Dir, ".astro"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(p.Dir, "Dockerfile"), "FROM quay.io/astronomer/astro-runtime:3.1-12\n")
	write(t, filepath.Join(p.Dir, ".astro", "config.yaml"), "project:\n  name: orders\n")
	write(t, filepath.Join(p.Dir, "requirements.txt"), "pandas\n")
	write(t, filepath.Join(p.Dir, "pyproject.toml"), "[tool.ruff]\nline-length = 120\n")

	p.run("dev", "pytest").
		requireFailure().
		requireStderr("Convert with astro init, then use uv run pytest").
		requireStderr("project made by Astro CLI 1.x (Dockerfile and .astro/)")

	var payload struct {
		Convert     string `json:"convert"`
		Is1xProject bool   `json:"v1_project"`
	}
	p.run("dev", "pytest", "--output", "json").requireFailure().requireJSON(&payload)
	if payload.Convert != "astro init" || !payload.Is1xProject {
		t.Errorf("convert = %q, v1_project = %v; want astro init, true", payload.Convert, payload.Is1xProject)
	}
}
