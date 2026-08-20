module github.com/astronomer/astro-cli/pkg/imagebuild

go 1.26.1

require (
	github.com/astronomer/astro-cli/pkg/localrt v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.12.0
)

require (
	github.com/astronomer/astro-cli/pkg/airflowrt v0.0.0-00010101000000-000000000000 // indirect
	github.com/astronomer/astro-cli/pkg/container v0.0.0-00010101000000-000000000000 // indirect
	github.com/astronomer/astro-cli/pkg/fsatomic v0.0.0-00010101000000-000000000000 // indirect
	github.com/astronomer/astro-cli/pkg/proxy v0.0.0-00010101000000-000000000000 // indirect
	github.com/astronomer/astro-cli/pkg/uv v0.0.0-00010101000000-000000000000 // indirect
	golang.org/x/sys v0.47.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/astronomer/astro-cli/pkg/localrt => ../localrt

// localrt's own in-repo deps, inherited by importing it. A cost of the direction:
// imagebuild depends on the contract for its progress types, so it also has to
// resolve everything the contract's engines need. The alternative — imagebuild
// declaring its own progress types so localrt could import IT instead — would keep
// this list empty. Worth revisiting if this set grows.
replace github.com/astronomer/astro-cli/pkg/airflowrt => ../airflowrt

replace github.com/astronomer/astro-cli/pkg/container => ../container

replace github.com/astronomer/astro-cli/pkg/fsatomic => ../fsatomic

replace github.com/astronomer/astro-cli/pkg/proxy => ../proxy

replace github.com/astronomer/astro-cli/pkg/uv => ../uv
