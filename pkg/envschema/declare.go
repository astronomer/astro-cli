package envschema

import (
	"fmt"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
)

// The two halves a writer of [tool.astro.env] needs from the grammar, kept here
// beside the parser so the writer cannot drift from the reader: the name a
// declaration is written under, and the TOML value it is written as. Neither
// does I/O; pkg/scaffold owns the file.

// DeclarationName is the name to declare key under in section, or the reason
// it cannot be declared.
//
// An Airflow variable or a connection may arrive in its env form, the way
// Environment Manager can store one (AIRFLOW_VAR_REGION, AIRFLOW_CONN_DB_MAIN).
// Declared verbatim, AIRFLOW_VAR_REGION would be the variable
// airflow_var_region, whose env key AIRFLOW_VAR_AIRFLOW_VAR_REGION matches
// nothing, so the declaration would succeed and bring in nothing. It is
// declared by its plain name instead (region, db_main).
//
// Variable keys and connection ids are lower-cased, because the AIRFLOW_VAR_
// and AIRFLOW_CONN_ round trip does: region and REGION are one variable to
// Airflow. An env var is used as written, since env var names are
// case-sensitive.
//
// The name is then checked by CheckName, the rule ParseSchema applies, because
// the parser refuses the whole section over one bad name.
func DeclarationName(section Section, key string) (string, error) {
	name, err := FoldName(section, key)
	if err != nil {
		return "", err
	}
	if err := CheckName(section, name); err != nil {
		return "", err
	}
	return name, nil
}

// FoldName is DeclarationName without the name check, for a caller that has to
// address a declaration whose name the grammar refuses: removing a bad name is
// how a section that no longer parses gets fixed.
func FoldName(section Section, key string) (string, error) {
	switch section {
	case SectionEnvVar:
		return key, nil
	case SectionAirflowVariable:
		if airflowenv.IsVarEnvKey(key) {
			key = strings.TrimPrefix(key, airflowenv.VarPrefix)
		}
		return strings.ToLower(key), nil
	case SectionConnection:
		return strings.ToLower(airflowenv.ConnIDForStoredConnKey(key)), nil
	default:
		return "", fmt.Errorf("%q is not a section of [tool.astro.env] (%s, %s, %s)",
			section, SectionEnvVar, SectionAirflowVariable, SectionConnection)
	}
}

// DeclarationTable renders spec as the table a writer puts under its name in
// section. ParseSchema reads the result back as the same declaration.
//
// It writes only what a reader would not already assume. `optional` is false
// unless written, and a connection is secret by its section: writing
// `secret` there is refused whichever value it carries, so it is never
// written for a connection, whatever spec.Secret says.
//
// It renders and does not judge. A spec Check refuses renders to a table
// ParseSchema refuses too, which is where a writer finds out: pkg/scaffold's
// EditManifest parses every result before writing it.
func DeclarationTable(spec *ValueSpec, section Section) map[string]any {
	out := map[string]any{}
	if spec.Source != "" {
		out["source"] = string(spec.Source)
	}
	if spec.Type != "" {
		out["type"] = string(spec.Type)
	}
	if spec.Optional {
		out["optional"] = true
	}
	if spec.Secret && section != SectionConnection {
		out["secret"] = true
	}
	if spec.HasDefault {
		out["default"] = spec.Default
	}
	if spec.ConnType != "" {
		out["conn_type"] = spec.ConnType
	}
	if spec.Description != "" {
		out["description"] = spec.Description
	}
	if len(spec.Enum) > 0 {
		enum := make([]any, len(spec.Enum))
		for i, v := range spec.Enum {
			enum[i] = v
		}
		out["enum"] = enum
	}
	return out
}
