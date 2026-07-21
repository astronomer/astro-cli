// Package manifest is the typed view of a project's pyproject.toml:
// [project] plus [tool.astro]. Load, validation, and the comment-preserving
// edit API land with an earlier fix; these types are the contract other v2 work
// builds against.
//
// This is a shared sub-module: Astro Desktop is expected to adopt the
// manifest as its project definition, so the leaf rules apply. One seam is
// deliberately open: the [tool.astro.env] schema section is validated by
// pkg/envschema, and sub-modules do not import each other — an earlier fix and
// an earlier fix decide together whether manifest re-exposes that section as its
// own types or the composition happens one layer up in each consumer.
package manifest

import "errors"

// Manifest is the parsed pyproject.toml, the parts astro reads.
type Manifest struct {
	Project Project
	Astro   Astro
}

// Project is the standard [project] table, the fields astro cares about.
type Project struct {
	Name           string
	RequiresPython string
	Dependencies   []string
}

// Astro is the [tool.astro] table.
type Astro struct {
	// AirflowVersion pins the Airflow the project runs and locks against.
	AirflowVersion string
	// Deployments is the committed deployment inventory,
	// [tool.astro.deployments.<name>].
	Deployments map[string]Deployment
}

// Deployment is one committed deployment link. Control-plane coordinates
// only — no URL, no credential; those are resolved at request time.
type Deployment struct {
	Target     string
	Workspace  string
	Deployment string
}

// ErrNotImplemented marks the contract stub below.
var ErrNotImplemented = errors.New("not yet implemented")

// Load reads and validates the manifest at path (a pyproject.toml).
// Implementation arrives with an earlier fix.
func Load(path string) (*Manifest, error) {
	return nil, ErrNotImplemented
}
