module github.com/astronomer/astro-cli/pkg/instances

go 1.26.1

// Three declared exceptions to the sibling rule (docs/v2-architecture.md), each
// on grounds an existing exception already uses.
//
// pkg/airflowrt and pkg/manifest are already direct desktop requires;
// pkg/airflowapi is not, so adopting this module adds one to the desktop's
// graph. Named rather than glossed, because "costs nothing" was the reason
// given for the exception the first time and it was not true.
//
//   - pkg/manifest: a link IS manifest data. Parsing it through a second
//     implementation would guarantee that resolving an instance and validating
//     the file disagree about what a link says — the reason pkg/scaffold has
//     the same exception.
//   - pkg/airflowapi: the vocabulary of what this package produces. It builds
//     a Transport and a CredentialSource; a parallel set of those types would
//     have to be converted at every boundary.
//   - pkg/airflowrt: the local Airflow's provisioned account and the directory
//     its standalone writes a generated password into. One owner for values
//     that have to move together — docker mode, standalone on macOS, and every
//     caller that authenticates.
//
// What is deliberately NOT here: the AWS SDK and Google's ADC chain. The two
// expensive auth doors are pkg/awsauth and pkg/googleauth, separate modules
// that import this one, so requiring the core costs neither in the binary nor
// in the module graph. That is the property TestTheAuthDoorsStayOptional holds.
require (
	github.com/astronomer/astro-cli/pkg/airflowapi v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/airflowrt v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/manifest v0.0.0-00010101000000-000000000000
)

require github.com/pelletier/go-toml/v2 v2.4.4-0.20260718201843-686c980c4758 // indirect

replace github.com/astronomer/astro-cli/pkg/airflowapi => ../airflowapi

replace github.com/astronomer/astro-cli/pkg/airflowrt => ../airflowrt

replace github.com/astronomer/astro-cli/pkg/manifest => ../manifest
