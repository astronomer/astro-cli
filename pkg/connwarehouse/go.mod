module github.com/astronomer/astro-cli/pkg/connwarehouse

go 1.26.1

// Declared exceptions to the sibling rule (docs/v2-architecture.md): the
// connection value type it maps, and the atomic write both files go through.
require (
	github.com/astronomer/astro-cli/pkg/connmodel v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/fsatomic v0.0.0-00010101000000-000000000000
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/astronomer/astro-cli/pkg/connmodel => ../connmodel

replace github.com/astronomer/astro-cli/pkg/fsatomic => ../fsatomic
