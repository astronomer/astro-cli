module github.com/astronomer/astro-cli/pkg/awsauth

go 1.26.1

// Declared exceptions to the sibling rule (docs/v2-architecture.md): a door
// implements pkg/instances' Provider, so it names that package's Instance,
// Deps and Provider types; it returns pkg/airflowapi's Transport and
// CredentialSource; and its tests use pkg/manifest to build an instance from
// the TOML a user would write. The dependency runs one way and the core never
// imports a door — which is the whole point, since importing this module is
// what puts the AWS SDK in a binary.
require (
	github.com/astronomer/astro-cli/pkg/airflowapi v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/instances v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/manifest v0.0.0-00010101000000-000000000000
	github.com/aws/aws-sdk-go-v2 v1.42.1
	github.com/aws/aws-sdk-go-v2/config v1.32.17
	github.com/aws/aws-sdk-go-v2/credentials v1.19.16
	github.com/aws/aws-sdk-go-v2/service/mwaa v1.41.7
)

require (
	github.com/astronomer/astro-cli/pkg/airflowrt v0.0.0-00010101000000-000000000000 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.23 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.24 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.23 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.21 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.42.1 // indirect
	github.com/aws/smithy-go v1.27.3 // indirect
	github.com/pelletier/go-toml/v2 v2.4.4-0.20260718201843-686c980c4758 // indirect
)

replace github.com/astronomer/astro-cli/pkg/airflowapi => ../airflowapi

replace github.com/astronomer/astro-cli/pkg/airflowrt => ../airflowrt

replace github.com/astronomer/astro-cli/pkg/instances => ../instances

replace github.com/astronomer/astro-cli/pkg/manifest => ../manifest
