package plan

import (
	"fmt"
	"strings"

	"github.com/astronomer/astro-cli/internal/envresolve"
	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/pkg/envschema"
)

// MissingEnvError reports that the project declares required environment
// values this machine has no source for. It is the clone-and-run gate: its
// message names each missing value and the exact `astro local env <noun> set`
// command that fills it, and Payload backs the same report in --output json.
// cmd decides how to render it.
type MissingEnvError struct {
	Project string
	Missing []envresolve.Missing
}

func (e *MissingEnvError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "this project needs %d environment value(s) that are not set on this machine:\n", len(e.Missing))
	for _, m := range e.Missing {
		fmt.Fprintf(&b, "  - %s %s\n", sectionLabel(m.Section), m.Name)
		if m.SourceNote != "" {
			fmt.Fprintf(&b, "      %s\n", m.SourceNote)
		}
		fmt.Fprintf(&b, "      provide it:  %s\n", setHint(&m))
	}
	b.WriteString("provide them, then run `astro local start` again — or start without them: `astro local start --allow-missing`.")
	return b.String()
}

// MissingValue is one unset value in a MissingPayload: what the resolver
// looked for, plus the exact command that would supply it.
type MissingValue struct {
	envresolve.Missing
	SetCommand string `json:"set_command"`
}

// MissingPayload is the --output json shape a start blocked on unset values
// publishes: the same data the message renders, structured for a coding
// agent to act on.
//
// Declared at package level rather than inside Payload so it is a type the
// schema pins in cmd/local can hold. Its own doc says it is for an agent to
// act on, which makes it exactly the kind of shape that must not drift
// unnoticed.
type MissingPayload struct {
	Error   string         `json:"error"`
	Project string         `json:"project"`
	Missing []MissingValue `json:"missing"`
}

// Payload is the --output json shape: the same data the message renders, plus
// the set command per value, structured for a coding agent to act on.
func (e *MissingEnvError) Payload() any {
	out := make([]MissingValue, 0, len(e.Missing))
	for i := range e.Missing {
		out = append(out, MissingValue{Missing: e.Missing[i], SetCommand: setHint(&e.Missing[i])})
	}
	return MissingPayload{
		Error:   "required environment values are not set on this machine",
		Project: e.Project,
		Missing: out,
	}
}

// setHint is the one hint form for every kind: the exact `astro local env <noun> set`
// command, scoped to the project (the default scope, so a copy-paste lands
// where the resolver looked).
//
// The noun comes from localenv rather than a switch here, so this hint and the
// command tree it names are renamed together or not at all.
func setHint(m *envresolve.Missing) string {
	noun := localenv.Noun(localenv.KindForSection(m.Section))
	return "astro local env " + noun + " set " + m.Name + " --project"
}

// sectionLabel is the human word for a schema section.
func sectionLabel(s envschema.Section) string {
	switch s {
	case envschema.SectionEnvVar:
		return "env var"
	case envschema.SectionAirflowVariable:
		return "airflow variable"
	case envschema.SectionConnection:
		return "connection"
	default:
		return string(s)
	}
}
