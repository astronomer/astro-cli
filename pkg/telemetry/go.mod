module github.com/astronomer/astro-cli/pkg/telemetry

go 1.26.10

require (
	github.com/google/uuid v1.6.0
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/astronomer/astro-cli/pkg/testenv v0.0.0-00010101000000-000000000000
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/astronomer/astro-cli/pkg/testenv => ../testenv
