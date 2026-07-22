module github.com/astronomer/astro-cli/pkg/airflowenv

go 1.26.1

// Declared exception to the sibling rule (docs/v2-architecture.md):
// the codec exists to encode the connmodel value type.
require github.com/astronomer/astro-cli/pkg/connmodel v0.0.0-00010101000000-000000000000

replace github.com/astronomer/astro-cli/pkg/connmodel => ../connmodel
