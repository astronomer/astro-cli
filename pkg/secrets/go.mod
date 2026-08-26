module github.com/astronomer/astro-cli/pkg/secrets

go 1.26.1

require github.com/zalando/go-keyring v0.2.8

require (
	github.com/astronomer/astro-cli/pkg/fsatomic v0.0.0-00010101000000-000000000000
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	golang.org/x/sys v0.27.0 // indirect
)

replace github.com/astronomer/astro-cli/pkg/fsatomic => ../fsatomic
