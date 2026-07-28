package plan

import (
	"fmt"
	"strings"

	"github.com/astronomer/astro-cli/internal/envresolve"
	"github.com/astronomer/astro-cli/pkg/envschema"
)

// MissingEnvError reports that the project declares required environment
// values this machine has no source for. It is the clone-and-run gate: its
// message names each missing value and the exact `astro local env set`
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
	b.WriteString("provide them, then run `astro local start` again.")
	return b.String()
}

// Payload is the --output json shape: the same data the message renders, plus
// the set command per value, structured for a coding agent to act on.
func (e *MissingEnvError) Payload() any {
	type missing struct {
		envresolve.Missing
		SetCommand string `json:"set_command"`
	}
	out := make([]missing, 0, len(e.Missing))
	for i := range e.Missing {
		out = append(out, missing{Missing: e.Missing[i], SetCommand: setHint(&e.Missing[i])})
	}
	return struct {
		Error   string    `json:"error"`
		Project string    `json:"project"`
		Missing []missing `json:"missing"`
	}{
		Error:   "required environment values are not set on this machine",
		Project: e.Project,
		Missing: out,
	}
}

// setHint is the one hint form for every kind: the exact `astro local env set`
// command, scoped to the project (the default scope, so a copy-paste lands
// where the resolver looked).
func setHint(m *envresolve.Missing) string {
	var args string
	switch m.Section {
	case envschema.SectionConnection:
		args = "conn " + m.Name
	case envschema.SectionAirflowVariable:
		args = "var " + m.Name
	case envschema.SectionEnvVar:
		args = m.Name
	}
	return "astro local env set " + args + " --project"
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
