module github.com/astronomer/astro-cli/pkg/scaffold

go 1.26.1

require (
	github.com/astronomer/astro-cli/pkg/manifest v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/uv v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/astronomer/astro-cli/pkg/airflowenv v0.0.0-00010101000000-000000000000 // indirect
	github.com/astronomer/astro-cli/pkg/connmodel v0.0.0-00010101000000-000000000000 // indirect
)

require (
	github.com/astronomer/astro-cli/pkg/envschema v0.0.0-00010101000000-000000000000
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pelletier/go-toml/v2 v2.4.4-0.20260718201843-686c980c4758 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/astronomer/astro-cli/pkg/manifest => ../manifest

replace github.com/astronomer/astro-cli/pkg/fsatomic => ../fsatomic

replace github.com/astronomer/astro-cli/pkg/uv => ../uv

replace github.com/astronomer/astro-cli/pkg/envschema => ../envschema

replace github.com/astronomer/astro-cli/pkg/airflowenv => ../airflowenv

replace github.com/astronomer/astro-cli/pkg/connmodel => ../connmodel
