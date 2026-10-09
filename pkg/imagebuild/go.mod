module github.com/astronomer/astro-cli/pkg/imagebuild

go 1.26.1

// Only the contract leaf, whose own dependency list is empty. This module used to
// require pkg/localrt itself, which meant inheriting the local runtime's engines
// (and airflowrt, container, fsatomic, proxy, uv, plus a replace line for each)
// to obtain four progress types.
require github.com/astronomer/astro-cli/pkg/localrt v0.0.0-00010101000000-000000000000

// Which requirements state the Airflow version, the one rule a generated
// build, a pre-flight check and a package share. A leaf over go-toml.
require github.com/astronomer/astro-cli/pkg/manifest v0.0.0-00010101000000-000000000000

// The runtime catalog, for the Airflow 2 image lookup. A stdlib-only leaf, so it
// brings no requires of its own; a consumer of this module still adds a
// require + replace pair for it, since a dependency's replace lines are never
// honoured.
require github.com/astronomer/astro-cli/pkg/runtimeversions v0.0.0-00010101000000-000000000000

// The Dockerfile secret-mount scan behind MissingBuildSecrets. A leaf with no
// requires of its own.
require github.com/astronomer/astro-cli/pkg/airflowrt v0.0.0-00010101000000-000000000000

require github.com/stretchr/testify v1.11.1

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pelletier/go-toml/v2 v2.4.4-0.20260718201843-686c980c4758 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/astronomer/astro-cli/pkg/localrt => ../localrt

replace github.com/astronomer/astro-cli/pkg/manifest => ../manifest

replace github.com/astronomer/astro-cli/pkg/runtimeversions => ../runtimeversions

replace github.com/astronomer/astro-cli/pkg/airflowrt => ../airflowrt
