module github.com/astronomer/astro-cli/pkg/scaffold

go 1.26.10

require (
	github.com/astronomer/astro-cli/pkg/airflowenv v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/airflowrt v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/connmodel v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/fsatomic v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/manifest v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/runtimeversions v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/uv v0.0.0-00010101000000-000000000000
	github.com/go-git/go-git/v5 v5.19.2
	github.com/moby/patternmatcher v0.6.1
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.9.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/zalando/go-keyring v0.2.8 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
)

require (
	github.com/astronomer/astro-cli/pkg/envschema v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/secrets v0.0.0
	github.com/astronomer/astro-cli/pkg/testenv v0.0.0-00010101000000-000000000000
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pelletier/go-toml/v2 v2.4.4-0.20260718201843-686c980c4758
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/astronomer/astro-cli/pkg/manifest => ../manifest

replace github.com/astronomer/astro-cli/pkg/runtimeversions => ../runtimeversions

replace github.com/astronomer/astro-cli/pkg/fsatomic => ../fsatomic

replace github.com/astronomer/astro-cli/pkg/uv => ../uv

replace github.com/astronomer/astro-cli/pkg/envschema => ../envschema

replace github.com/astronomer/astro-cli/pkg/airflowenv => ../airflowenv

replace github.com/astronomer/astro-cli/pkg/connmodel => ../connmodel

replace github.com/astronomer/astro-cli/pkg/secrets => ../secrets

replace github.com/astronomer/astro-cli/pkg/airflowrt => ../airflowrt

replace github.com/astronomer/astro-cli/pkg/testenv => ../testenv
