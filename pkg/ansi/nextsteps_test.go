package ansi

import (
	"fmt"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/input"
)

func nextStepsText(n NextSteps) string {
	var b strings.Builder
	PrintNextSteps(&b, n)
	return b.String()
}

func TestPrintNextStepsLayout(t *testing.T) {
	steps := func(n int) []NextStep {
		out := make([]NextStep, n)
		for i := range out {
			out[i] = NextStep{Why: fmt.Sprintf("Set V%d so astro local start can run", i), Command: fmt.Sprintf("astro local env variable set V%d", i)}
		}
		return out
	}
	tests := []struct {
		name string
		in   NextSteps
		want string
	}{
		{"none", NextSteps{}, ""},
		{
			"one command with no reason stays on the label's line",
			NextSteps{Steps: []NextStep{{Command: "astro local start"}}},
			"\nNext: astro local start\n",
		},
		{
			"a reason puts the command on a line of its own",
			NextSteps{Steps: steps(1)},
			"\nNext:\n  Set V0 so astro local start can run:\n    astro local env variable set V0\n",
		},
		{
			"several commands with no reason are listed under the label",
			NextSteps{Steps: []NextStep{{Command: "astro a"}, {Command: "astro b"}}},
			"\nNext:\n  astro a\n  astro b\n",
		},
		{
			"past the cap the rest are counted, and More says where they are",
			NextSteps{Steps: steps(7), More: NextStep{Why: "marked in the STATUS column above"}},
			"\nNext:\n" +
				"  Set V0 so astro local start can run:\n    astro local env variable set V0\n" +
				"  Set V1 so astro local start can run:\n    astro local env variable set V1\n" +
				"  Set V2 so astro local start can run:\n    astro local env variable set V2\n" +
				"  Set V3 so astro local start can run:\n    astro local env variable set V3\n" +
				"  Set V4 so astro local start can run:\n    astro local env variable set V4\n" +
				"  and 2 more, marked in the STATUS column above.\n",
		},
		{
			"More can name the command that lists them all",
			NextSteps{Steps: steps(6), More: NextStep{Why: "list them all", Command: "astro local env list -o json"}},
			"  and 1 more, list them all:\n    astro local env list -o json\n",
		},
		{
			"with no More the count stands alone",
			NextSteps{Steps: steps(6)},
			"  and 1 more.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextStepsText(tt.in)
			if strings.HasPrefix(tt.want, "  and") {
				if !strings.HasSuffix(got, tt.want) {
					t.Errorf("got\n%q\nwant it to end with\n%q", got, tt.want)
				}
				return
			}
			if got != tt.want {
				t.Errorf("got\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// Optional steps are for a person at a terminal who has not opted out: a
// stderr that is no terminal, or ASTRO_GUIDANCE_DISABLED, hides them. A
// required step prints regardless.
func TestPrintNextStepsGate(t *testing.T) {
	optional := NextSteps{Steps: []NextStep{{Command: "astro local start"}}}
	required := NextSteps{Steps: []NextStep{{Command: "astro deploy --dags"}}, Required: true}

	if got := nextStepsText(optional); got == "" {
		t.Fatal("an optional step should print on a terminal")
	}

	for _, v := range []string{"1", "true", "TRUE"} {
		t.Run("guidance disabled "+v, func(t *testing.T) {
			t.Setenv(GuidanceDisabledEnv, v)
			if got := nextStepsText(optional); got != "" {
				t.Errorf("%s=%s should hide an optional step: %q", GuidanceDisabledEnv, v, got)
			}
			if got := nextStepsText(required); got != "\nNext: astro deploy --dags\n" {
				t.Errorf("a required step should still print: %q", got)
			}
		})
	}
	t.Run("guidance not disabled by other values", func(t *testing.T) {
		t.Setenv(GuidanceDisabledEnv, "0")
		if got := nextStepsText(optional); got == "" {
			t.Errorf("%s=0 should leave optional steps on", GuidanceDisabledEnv)
		}
	})

	t.Run("stderr not a terminal", func(t *testing.T) {
		restore := input.SetTerminals(true, false)
		defer restore()
		if got := nextStepsText(optional); got != "" {
			t.Errorf("an optional step should not print without a terminal: %q", got)
		}
		if got := nextStepsText(required); got == "" {
			t.Error("a required step should print without a terminal")
		}
	})
}
