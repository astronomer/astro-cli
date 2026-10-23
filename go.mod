module github.com/astronomer/astro-cli

go 1.26.1

tool (
	github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen
	github.com/vektra/mockery/v2
)

require (
	github.com/Masterminds/semver/v3 v3.4.0
	github.com/astronomer/astro-cli/pkg/telemetry v0.0.0
	github.com/briandowns/spinner v1.23.1
	github.com/docker/docker v28.5.1+incompatible
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/logrusorgru/aurora v2.0.3+incompatible
	github.com/lucsky/cuid v1.2.1
	github.com/mattn/go-isatty v0.0.20
	github.com/mitchellh/go-homedir v1.1.0
	github.com/moby/buildkit v0.25.1
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.9.4
	github.com/spf13/afero v1.11.0
	github.com/spf13/cobra v1.10.2
	github.com/spf13/viper v1.19.0
	github.com/stretchr/testify v1.12.0
	golang.org/x/net v0.58.0
	golang.org/x/sys v0.47.0
)

require (
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.7.0
	github.com/aquasecurity/go-pep440-version v0.0.1
	github.com/astronomer/astro-cli/pkg/airflowapi v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/airflowenv v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/airflowrt v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/astroauth v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/checks v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/connmodel v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/connwarehouse v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/container v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/envschema v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/fsatomic v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/imagebuild v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/localrt v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/proxy v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/runtimeversions v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/secrets v0.0.0
	github.com/astronomer/astro-cli/pkg/uv v0.0.0-00010101000000-000000000000
	github.com/distribution/reference v0.6.0
	github.com/fatih/camelcase v1.0.0
	github.com/ghodss/yaml v1.0.0
	github.com/go-git/go-billy/v5 v5.9.0
	github.com/go-git/go-git/v5 v5.19.2
	github.com/golang-jwt/jwt/v4 v4.5.2
	github.com/itchyny/gojq v0.12.18
	github.com/mitchellh/mapstructure v1.5.0
	github.com/moby/patternmatcher v0.6.1
	github.com/neilotoole/jsoncolor v0.7.1
	github.com/oapi-codegen/runtime v1.4.0
	github.com/pb33f/libopenapi v0.34.0
	github.com/pelletier/go-toml/v2 v2.4.4-0.20260718201843-686c980c4758
	github.com/whilp/git-urls v1.0.0
	github.com/zalando/go-keyring v0.2.8
	go.yaml.in/yaml/v4 v4.0.0-rc.4
	golang.org/x/exp v0.0.0-20260410095643-746e56fc9e2f
	golang.org/x/mod v0.38.0
	golang.org/x/term v0.45.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.21.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.12.0 // indirect
	github.com/aquasecurity/go-version v0.0.1 // indirect
	github.com/aws/aws-sdk-go-v2 v1.42.1 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.32.17 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.19.16 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.23 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.24 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.23 // indirect
	github.com/aws/aws-sdk-go-v2/service/mwaa v1.41.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.21 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.42.1 // indirect
	github.com/aws/smithy-go v1.27.3 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.1.2 // indirect
	github.com/chigopher/pathlib v0.19.1 // indirect
	github.com/containerd/typeurl/v2 v2.2.3 // indirect
	github.com/cyphar/filepath-securejoin v0.6.1 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/dprotaso/go-yit v0.0.0-20220510233725-9ba8df137936 // indirect
	github.com/getkin/kin-openapi v0.149.0 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-openapi/jsonpointer v0.22.5 // indirect
	github.com/go-openapi/swag/jsonname v0.25.5 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/huandu/xstrings v1.5.0 // indirect
	github.com/itchyny/timefmt-go v0.1.7 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/nxadm/tail v1.4.11 // indirect
	github.com/oapi-codegen/oapi-codegen/v2 v2.7.1 // indirect
	github.com/oasdiff/yaml v0.1.1 // indirect
	github.com/oasdiff/yaml3 v0.0.14 // indirect
	github.com/onsi/gomega v1.34.2 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/pb33f/jsonpath v0.8.1 // indirect
	github.com/pb33f/ordered-map/v2 v2.3.0 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/rs/zerolog v1.33.0 // indirect
	github.com/sagikazarmark/locafero v0.6.0 // indirect
	github.com/sagikazarmark/slog-shim v0.1.0 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/speakeasy-api/jsonpath v0.6.3 // indirect
	github.com/speakeasy-api/openapi v1.19.2 // indirect
	github.com/vektra/mockery/v2 v2.50.0 // indirect
	github.com/vmware-labs/yaml-jsonpath v0.3.2 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/tools v0.48.0 // indirect
	golang.org/x/xerrors v0.0.0-20231012003039-104605ab7028 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
)

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/astronomer/astro-cli/pkg/awsauth v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/emfetch v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/googleauth v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/instancelocate v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/instances v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/manifest v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/platformversions v0.0.0-00010101000000-000000000000
	github.com/astronomer/astro-cli/pkg/scaffold v0.0.0-00010101000000-000000000000
	github.com/fatih/color v1.18.0
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/gofrs/flock v0.13.0
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/magiconair/properties v1.8.9 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/spf13/cast v1.7.0 // indirect
	github.com/spf13/pflag v1.0.10
	github.com/stretchr/objx v0.5.3 // indirect
	github.com/subosito/gotenv v1.6.0
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v2 v2.4.0
)

replace github.com/astronomer/astro-cli/pkg/proxy => ./pkg/proxy

replace github.com/astronomer/astro-cli/pkg/airflowrt => ./pkg/airflowrt

replace github.com/astronomer/astro-cli/pkg/container => ./pkg/container

replace github.com/astronomer/astro-cli/pkg/astroauth => ./pkg/astroauth

replace github.com/astronomer/astro-cli/pkg/telemetry => ./pkg/telemetry

replace github.com/astronomer/astro-cli/pkg/localrt => ./pkg/localrt

replace github.com/astronomer/astro-cli/pkg/fsatomic => ./pkg/fsatomic

replace github.com/astronomer/astro-cli/pkg/imagebuild => ./pkg/imagebuild

replace github.com/astronomer/astro-cli/pkg/runtimeversions => ./pkg/runtimeversions

replace github.com/astronomer/astro-cli/pkg/manifest => ./pkg/manifest

replace github.com/astronomer/astro-cli/pkg/uv => ./pkg/uv

replace github.com/astronomer/astro-cli/pkg/envschema => ./pkg/envschema

replace github.com/astronomer/astro-cli/pkg/secrets => ./pkg/secrets

replace github.com/astronomer/astro-cli/pkg/checks => ./pkg/checks

replace github.com/astronomer/astro-cli/pkg/connmodel => ./pkg/connmodel

replace github.com/astronomer/astro-cli/pkg/connwarehouse => ./pkg/connwarehouse

replace github.com/astronomer/astro-cli/pkg/platformversions => ./pkg/platformversions

replace github.com/astronomer/astro-cli/pkg/airflowenv => ./pkg/airflowenv

replace github.com/astronomer/astro-cli/pkg/airflowapi => ./pkg/airflowapi

replace github.com/astronomer/astro-cli/pkg/scaffold => ./pkg/scaffold

replace github.com/astronomer/astro-cli/pkg/instancelocate => ./pkg/instancelocate

replace github.com/astronomer/astro-cli/pkg/instances => ./pkg/instances

replace github.com/astronomer/astro-cli/pkg/awsauth => ./pkg/awsauth

replace github.com/astronomer/astro-cli/pkg/googleauth => ./pkg/googleauth

replace github.com/astronomer/astro-cli/pkg/emfetch => ./pkg/emfetch
