package scaffold

import (
	"sort"

	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// setEnvDeclarations writes the declarations airflow_settings.yaml carried
// into [tool.astro.env].
//
// Leaf by leaf rather than as one table, so the section reads as a person would
// write it and a later edit lands beside these rather than replacing them.
//
// There is no "unless the manifest already declares this name" case, and the
// reason is worth stating because the guard looks obviously needed:
// [tool.astro.env] cannot exist without tool.astro existing, and a manifest
// carrying that has already been refused with ErrAlreadyAstroProject. This is
// the same dead branch mergeDependencies' packages arm once carried, so it is
// left out rather than written and explained.
func setEnvDeclarations(ed tomledit.Editor, c *carriedSettings) error {
	if !c.declares() {
		return nil
	}
	for _, sec := range []struct {
		path    []string
		section envschema.Section
		specs   map[string]envschema.ValueSpec
	}{
		{[]string{"tool", "astro", "env"}, envschema.SectionEnvVar, c.schema.EnvVars},
		{[]string{"tool", "astro", "env", "airflow_variables"}, envschema.SectionAirflowVariable, c.schema.AirflowVariables},
		{[]string{"tool", "astro", "env", "connections"}, envschema.SectionConnection, c.schema.Connections},
	} {
		for _, name := range sortedSpecNames(sec.specs) {
			path := append(append([]string{}, sec.path...), name)
			spec := sec.specs[name]
			if err := ed.Set(path, envschema.DeclarationTable(&spec, sec.section)); err != nil {
				return err
			}
		}
	}
	return nil
}

func sortedSpecNames(m map[string]envschema.ValueSpec) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
