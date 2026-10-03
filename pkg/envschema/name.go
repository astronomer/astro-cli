package envschema

import (
	"fmt"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
)

// CheckName reports why a declared NAME cannot be written under
// [tool.astro.env], or nil when it can.
//
// Separate from ValueSpec.Check, which judges a declaration's annotations and
// never sees what it is called. Both have to pass before anything writes: the
// parser refuses an illegal name, and it refuses the WHOLE section over one of
// them — so a single bad name makes every other declaration in the project
// unreadable, and a caller that deletes its source first has destroyed the only
// copy.
//
// The rule is the parser's own, reached through the same package, so the two
// cannot drift.
func CheckName(section Section, name string) error {
	switch section {
	case SectionConnection:
		if !airflowenv.ValidConnID(name) {
			return fmt.Errorf("%q is not a valid connection id (letters, digits, _)", name)
		}
	case SectionAirflowVariable:
		if !airflowenv.ValidVarKey(name) {
			return fmt.Errorf("%q is not a valid Airflow Variable key (%s). Rename the variable, and the Dags reading it", name, airflowenv.VarKeyRule)
		}
	case SectionEnvVar:
		if !airflowenv.ValidEnvKey(name) {
			return fmt.Errorf("%q is not a legal env-var name (letters, digits, _; no leading digit)", name)
		}
		// A plain env var sits directly under [tool.astro.env], where these two
		// name the sub-sections. One called either is not expressible: the
		// parser reads the name as a section, and a writer would put the var's
		// own annotation keys where connection or variable declarations go.
		if name == sectionKeyConnections || name == sectionKeyAirflowVariables {
			return fmt.Errorf("%q is a reserved section name in [tool.astro.env] and cannot be an env var", name)
		}
	}
	return nil
}

// The two sub-section keys of [tool.astro.env].
const (
	sectionKeyConnections      = "connections"
	sectionKeyAirflowVariables = "airflow_variables"
)
