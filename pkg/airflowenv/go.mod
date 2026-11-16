module github.com/astronomer/astro-cli/pkg/airflowenv

go 1.26.10

require (
	// Declared exception to the sibling rule (docs/architecture.md):
	// the codec exists to encode the connmodel value type.
	github.com/astronomer/astro-cli/pkg/connmodel v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/testenv v0.0.0-00010101000000-000000000000
)

replace github.com/astronomer/astro-cli/pkg/connmodel => ../connmodel

replace github.com/astronomer/astro-cli/pkg/testenv => ../testenv
