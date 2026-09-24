package scaffold

import (
	"sort"

	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// A v1 project can declare the environment its Airflow expects in
// .astro/env.schema.yaml. v2 declares the same thing in [tool.astro.env], so
// the conversion carries it and the file retires with the other three.
//
// # All of it moves, or none of it does
//
// A partial carry has nowhere safe to put the remainder. Writing SOME
// declarations creates [tool.astro.env], and a manifest carrying that section
// is the project's declaration source from then on — so the file kept to
// preserve what did not fit is a file nothing will ever read again, and the
// declarations left in it have silently stopped applying. Keeping the file is
// only protection while the file still speaks.
//
// So a fault anywhere means nothing is written and the file stays whole and
// authoritative, exactly as it was. The project is not converted-and-damaged:
// it is converted, still reading its v1 file, with a note naming every fault so
// one pass fixes them all.
//
// # A `default` changes meaning
//
// In the v1 file it is documentation the desktop never applied; in the manifest
// it is composed into the environment at start, by the app and by
// `astro local start` alike. It is carried, and it draws a note, because the
// project starts behaving differently and nothing else on screen would show it.

// carriedEnvSchema is what a v1 file yielded.
type carriedEnvSchema struct {
	// schema is what to write, and it is either everything the file declared or
	// nothing at all. See the all-or-nothing note above.
	schema *envschema.Schema
	// blockers are the faults that stopped the carry. Each names the v1 file,
	// which is what keeps the file from retiring — see planRetirements.
	blockers []string
	// advisories describe something that WAS carried and now behaves
	// differently. They deliberately do not name a file, because they must not
	// stop one retiring; Plan appends them after the retirement decision, so a
	// declaration whose name happens to match a filename cannot spare it.
	advisories []string
}

// readEnvSchema decides whether the file can be carried, and reports every
// reason it cannot.
//
// Every reason, not the first: this feeds a preview of a destructive operation,
// so a user who fixes what it names should not be told about a second fault on
// the next run.
//
// A file that will not parse is not an error either. Failing the run would
// leave a user unable to convert until they hand-fixed a file the conversion
// was going to delete.
func readEnvSchema(data []byte) carriedEnvSchema {
	parsed, err := envschema.ParseLegacy(data)
	if err != nil {
		return carriedEnvSchema{blockers: []string{
			envschema.LegacyRelPath + " was not carried into [tool.astro.env], and is kept as it is. " + err.Error(),
		}}
	}

	var blockers, advisories []string
	for _, sec := range []struct {
		section envschema.Section
		specs   map[string]envschema.ValueSpec
	}{
		{envschema.SectionEnvVar, parsed.EnvVars},
		{envschema.SectionAirflowVariable, parsed.AirflowVariables},
		{envschema.SectionConnection, parsed.Connections},
	} {
		for _, name := range sortedSpecNames(sec.specs) {
			spec := sec.specs[name]
			// The name and the annotations are separate rules, and the parser
			// refuses the WHOLE section over either, so both run before
			// anything is written.
			if nerr := envschema.CheckName(sec.section, name); nerr != nil {
				blockers = append(blockers, envschema.LegacyRelPath+": "+nerr.Error())
			}
			for _, p := range spec.Check(sec.section) {
				blockers = append(blockers, envschema.LegacyRelPath+": "+name+" cannot be carried. "+p.Reason)
			}
			if spec.HasDefault {
				advisories = append(advisories, name+
					": its default is now composed into the environment at start. "+
					"In the v1 file a default was documentation and was never applied")
			}
		}
	}
	if len(blockers) > 0 {
		return carriedEnvSchema{blockers: blockers}
	}
	return carriedEnvSchema{schema: parsed, advisories: advisories}
}

// declares reports whether anything survived to be written.
func (c carriedEnvSchema) declares() bool {
	if c.schema == nil {
		return false
	}
	return len(c.schema.EnvVars)+len(c.schema.AirflowVariables)+len(c.schema.Connections) > 0
}

// setEnvDeclarations writes the carried declarations into [tool.astro.env].
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
func setEnvDeclarations(ed tomledit.Editor, c carriedEnvSchema) error {
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
