module github.com/astronomer/astro-cli

go 1.13

replace github.com/docker/docker => github.com/docker/engine v0.0.0-20190725163905-fa8dd90ceb7b

replace github.com/Nvveen/Gotty v0.0.0 => github.com/ijc25/Gotty v0.0.0-20170406111628-a8b993ba6abd

replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible

require (
	github.com/Microsoft/go-winio v0.4.7 // indirect
	github.com/Nvveen/Gotty v0.0.0 // indirect
	github.com/agl/ed25519 v0.0.0-20170116200512-5312a6153412 // indirect
	github.com/containerd/containerd v1.2.9 // indirect
	github.com/containerd/fifo v0.0.0-20190816180239-bda0ff6ed73c // indirect
	github.com/containerd/typeurl v0.0.0-20190515163108-7312978f2987 // indirect
	github.com/docker/cli v0.0.0-20190711175710-5b38d82aa076
	github.com/docker/docker v0.0.0-00010101000000-000000000000
	github.com/docker/go v1.5.1-1 // indirect
	github.com/docker/go-connections v0.4.0
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/docker/libcompose v0.4.1-0.20190808084053-143e0f3f1ab9
	github.com/fatih/camelcase v1.0.0
	github.com/gogo/googleapis v1.3.0 // indirect
	github.com/gorilla/mux v1.6.1 // indirect
	github.com/gorilla/websocket v1.4.0
	github.com/hashicorp/go-version v1.2.0 // indirect
	github.com/iancoleman/strcase v0.0.0-20171129010253-3de563c3dc08
	github.com/ijc25/Gotty v0.0.0-20170406111628-a8b993ba6abd // indirect
	github.com/miekg/pkcs11 v1.0.2 // indirect
	github.com/mitchellh/go-homedir v1.1.0
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/opencontainers/runc v0.1.1 // indirect
	github.com/opencontainers/runtime-spec v1.0.1 // indirect
	github.com/pkg/errors v0.8.0
	github.com/shurcooL/go v0.0.0-20180410215514-47fa5b7ceee6
	github.com/spf13/cobra v0.0.5
	github.com/spf13/viper v1.3.2
	github.com/syndtr/gocapability v0.0.0-20180916011248-d98352740cb2 // indirect
	github.com/theupdateframework/notary v0.6.1 // indirect
	golang.org/x/crypto v0.0.0-20190308221718-c2843e01d9a2
	golang.org/x/text v0.3.1-0.20171227012246-e19ae1496984 // indirect
	gopkg.in/yaml.v2 v2.2.3-0.20190319135612-7b8349ac747c // indirect
	vbom.ml/util v0.0.0-20180919145318-efcd4e0f9787 // indirect
)
