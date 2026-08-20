module github.com/astronomer/astro-cli/pkg/imagebuild

go 1.26.1

// Only the contract leaf, whose own dependency list is empty. This module used to
// require pkg/localrt itself, which meant inheriting the local runtime's engines
// (and airflowrt, container, fsatomic, proxy, uv, plus a replace line for each)
// to obtain four progress types.
require github.com/astronomer/astro-cli/pkg/localrt v0.0.0-00010101000000-000000000000

require github.com/stretchr/testify v1.11.1

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/astronomer/astro-cli/pkg/localrt => ../localrt
