module github.com/astronomer/astro-cli/pkg/instancelocate

go 1.26.1

// Declared exceptions to the sibling rule (docs/architecture.md): the
// lookup takes and returns pkg/instances' Instance, which is the link it
// resolves; it reaches pkg/googleauth for the Application Default Credentials
// chain a Composer lookup runs under, plus the account advice its 403 carries,
// and takes that package's own Options rather than restating its fields; and —
// test-only — it builds instances through pkg/instances/instancestest, the
// shared preamble in a module it already requires.
//
// The googleauth dependency is the point rather than an accident: a Composer
// address cannot be read without Google credentials, so a consumer of this
// module is already paying for that chain. It is the AWS SDK that must not
// arrive here, which is what archlint's instancelocate clause holds.
require (
	github.com/astronomer/astro-cli/pkg/googleauth v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/instances v0.0.0-00010101000000-000000000000
)

require github.com/astronomer/astro-cli/pkg/manifest v0.0.0-00010101000000-000000000000 // indirect

require (
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/astronomer/astro-cli/pkg/airflowapi v0.0.0-00010101000000-000000000000 // indirect
	github.com/astronomer/astro-cli/pkg/airflowrt v0.0.0-00010101000000-000000000000 // indirect
	github.com/pelletier/go-toml/v2 v2.4.4-0.20260718201843-686c980c4758 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
)

replace github.com/astronomer/astro-cli/pkg/googleauth => ../googleauth

replace github.com/astronomer/astro-cli/pkg/instances => ../instances

replace github.com/astronomer/astro-cli/pkg/airflowapi => ../airflowapi

replace github.com/astronomer/astro-cli/pkg/airflowrt => ../airflowrt

replace github.com/astronomer/astro-cli/pkg/manifest => ../manifest
