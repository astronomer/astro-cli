module github.com/astronomer/astro-cli/pkg/manifest

go 1.26.10

require (
	github.com/astronomer/astro-cli/pkg/testenv v0.0.0-00010101000000-000000000000
	github.com/pelletier/go-toml/v2 v2.4.4-0.20260718201843-686c980c4758
)

replace github.com/astronomer/astro-cli/pkg/testenv => ../testenv
