package plan

import (
	"fmt"
	"strings"

	"github.com/astronomer/astro-cli/internal/envresolve"
	"github.com/astronomer/astro-cli/pkg/envschema"
)

// MissingEnvError reports that the project declares required environment
// values this machine has no source for. It is the clone-and-run gate: its
// message names each missing value and how to provide it, and Payload backs
// the same report in --output json. cmd decides how to render it.
type MissingEnvError struct {
	Project string
	Missing []envresolve.Missing
	// VaultUnavailable is set when the OS keyring could not be reached, so a
	// value may already exist that this machine cannot read.
	VaultUnavailable bool
}

func (e *MissingEnvError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "this project needs %d environment value(s) that are not set on this machine:\n", len(e.Missing))
	for _, m := range e.Missing {
		fmt.Fprintf(&b, "  - %s %s", sectionLabel(m.Section), m.Name)
		if m.Description != "" {
			fmt.Fprintf(&b, " — %s", m.Description)
		}
		fmt.Fprintf(&b, "\n      set %s, or store it in the vault under %s\n", m.EnvKey, m.VaultKey)
	}
	if e.VaultUnavailable {
		b.WriteString("(the OS keyring is unavailable here, so vault-stored values cannot be read)\n")
	}
	b.WriteString("provide them, then run `astro local start` again.")
	return b.String()
}

// Payload is the --output json shape: the same data the message renders,
// structured for a coding agent to act on.
func (e *MissingEnvError) Payload() any {
	return struct {
		Error            string               `json:"error"`
		Project          string               `json:"project"`
		VaultUnavailable bool                 `json:"vault_unavailable,omitempty"`
		Missing          []envresolve.Missing `json:"missing"`
	}{
		Error:            "required environment values are not set on this machine",
		Project:          e.Project,
		VaultUnavailable: e.VaultUnavailable,
		Missing:          e.Missing,
	}
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
