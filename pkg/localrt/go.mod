module github.com/astronomer/astro-cli/pkg/localrt

go 1.26.1

// The engines under internal/ need these. All five are declared in
// docs/v2-architecture.md's exception list (localrt -> airflowrt predates this
// change; proxy, container, uv, and fsatomic were added with it) — localrt
// orchestrates shared primitives, and re-implementing route publishing, engine
// detection, venv management, or atomic writes inside it would be worse than the
// version coordination these cost.
require (
	github.com/astronomer/astro-cli/pkg/airflowrt v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/container v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/fsatomic v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/proxy v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/uv v0.0.0-00010101000000-000000000000
)

require (
	github.com/stretchr/testify v1.11.1
	golang.org/x/sys v0.47.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
)

replace github.com/astronomer/astro-cli/pkg/airflowrt => ../airflowrt

replace github.com/astronomer/astro-cli/pkg/container => ../container

replace github.com/astronomer/astro-cli/pkg/fsatomic => ../fsatomic

replace github.com/astronomer/astro-cli/pkg/proxy => ../proxy

replace github.com/astronomer/astro-cli/pkg/uv => ../uv
