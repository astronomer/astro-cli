module github.com/astronomer/astro-cli/pkg/googleauth

go 1.26.1

// Declared exceptions to the sibling rule (docs/architecture.md): a door
// implements pkg/instances' Provider, so it names that package's Instance,
// Deps and Provider types; it returns pkg/airflowapi's Transport and
// CredentialSource; and — test-only — it names pkg/manifest's auth method to
// key itself into a Providers map, and builds its instances through
// pkg/instances/instancestest, the shared preamble in the module it already
// requires. The dependency runs one way and the core never
// imports a door — which is the whole point, since importing this module is
// what puts Google's application-default chain in a binary.
require (
	github.com/astronomer/astro-cli/pkg/airflowapi v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/instances v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/manifest v0.0.0-00010101000000-000000000000
	golang.org/x/oauth2 v0.36.0
)

require (
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/astronomer/astro-cli/pkg/airflowrt v0.0.0-00010101000000-000000000000 // indirect
	github.com/pelletier/go-toml/v2 v2.4.4-0.20260718201843-686c980c4758 // indirect
)

require golang.org/x/sys v0.35.0 // indirect

replace github.com/astronomer/astro-cli/pkg/airflowapi => ../airflowapi

replace github.com/astronomer/astro-cli/pkg/airflowrt => ../airflowrt

replace github.com/astronomer/astro-cli/pkg/instances => ../instances

replace github.com/astronomer/astro-cli/pkg/manifest => ../manifest
