module github.com/astronomer/astro-cli/pkg/checks

go 1.26.1

// Declared exception to the sibling rule (docs/v2-architecture.md): the
// target-aware pre-flight check picks which Airflow version to check against
// from pkg/platformversions, the shared table of what MWAA and Composer offer.
// A copy of that table would be a second answer to which versions exist, and
// it is a pure leaf, so nothing is linked for it beyond the table.
//
// It is not free to ADOPT, though: a dependency's own replace lines are never
// honoured, so anything requiring this module must add a require + replace pair
// for pkg/platformversions as well.
require github.com/astronomer/astro-cli/pkg/platformversions v0.0.0-00010101000000-000000000000

// The second exception: pkg/manifest, for which requirements state the Airflow
// version (manifest.WithoutAirflow, NamesAirflow), the rule a generated build
// and a package share. A leaf over go-toml; the same require + replace cost.
require github.com/astronomer/astro-cli/pkg/manifest v0.0.0-00010101000000-000000000000

require github.com/stretchr/testify v1.12.0

require (
	github.com/pelletier/go-toml/v2 v2.4.4-0.20260718201843-686c980c4758 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/astronomer/astro-cli/pkg/platformversions => ../platformversions

replace github.com/astronomer/astro-cli/pkg/manifest => ../manifest
