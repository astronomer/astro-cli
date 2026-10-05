module github.com/astronomer/astro-cli/pkg/envschema

go 1.26.1

// Declared exception to the sibling rule (docs/architecture.md): the parser
// validates declared names against airflowenv's three predicates, and
// DeclaredEnvKeys applies its AIRFLOW_VAR_/AIRFLOW_CONN_ encoding.
//
// go-toml is pinned to the version the root module and pkg/manifest use, not
// the newest release: parse_test.go decodes fixtures the way pkg/manifest hands
// them over, so a different unmarshaler here would pin a different decoder's
// answer than production gets.
require (
	github.com/astronomer/astro-cli/pkg/airflowenv v0.0.0-00010101000000-000000000000
	github.com/pelletier/go-toml/v2 v2.4.4-0.20260718201843-686c980c4758
)

// Indirect through airflowenv, and required here because a dependency's own
// replace lines are never honoured — only the main module's count.

require github.com/astronomer/astro-cli/pkg/connmodel v0.0.0-00010101000000-000000000000 // indirect

replace github.com/astronomer/astro-cli/pkg/airflowenv => ../airflowenv

replace github.com/astronomer/astro-cli/pkg/connmodel => ../connmodel
