package main

import (
	"os"

	"github.com/astronomer/astro-cli/cmd"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/spf13/afero"
)

//go:generate go run github.com/deepmap/oapi-codegen/cmd/oapi-codegen --version
//go:generate go run github.com/deepmap/oapi-codegen/cmd/oapi-codegen -include-tags=User,Organization,Invite,Workspace,Cluster,Options,Team,ApiToken,Deployment,Deploy,Environment -generate=types,client -package=astrocore -o=./astro-client-core/api.gen.go ../astro/apps/core/docs/public/v1alpha1/public_v1alpha1.yaml
//go:generate go run github.com/deepmap/oapi-codegen/cmd/oapi-codegen -include-tags=User,Invite,Team,ApiToken -generate=types,client -package=astroiamcore -o=./astro-client-iam-core/api.gen.go ../astro/apps/core/docs/iam/v1beta1/iam_v1beta1.yaml
//go:generate go run github.com/deepmap/oapi-codegen/cmd/oapi-codegen -include-tags=Organization,Workspace,Cluster,Options,Deployment -generate=types,client -package=astroplatformcore -o=./astro-client-platform-core/api.gen.go ../astro/apps/core/docs/platform/v1beta1/platform_v1beta1.yaml

//go:generate go run github.com/vektra/mockery/v2 --version
//go:generate go run github.com/vektra/mockery/v2 --filename=ClientInterface.go --output=houston/mocks --dir=houston --outpkg=houston_mocks --name=ClientInterface
//go:generate go run github.com/vektra/mockery/v2 --filename=RegistryHandler.go --output=airflow/mocks --dir=airflow --outpkg=mocks --name=RegistryHandler
//go:generate go run github.com/vektra/mockery/v2 --filename=ImageHandler.go --output=airflow/mocks --dir=airflow --outpkg=mocks --name=ImageHandler
//go:generate go run github.com/vektra/mockery/v2 --filename=ContainerHandler.go --output=airflow/mocks --dir=airflow --outpkg=mocks --name=ContainerHandler
//go:generate go run github.com/vektra/mockery/v2 --filename=DockerComposeAPI.go --output=airflow/mocks --dir=airflow --outpkg=mocks --name=DockerComposeAPI
//go:generate go run github.com/vektra/mockery/v2 --filename=DockerRegistryAPI.go --output=airflow/mocks --dir=airflow --outpkg=mocks --name=DockerRegistryAPI
//go:generate go run github.com/vektra/mockery/v2 --filename=DockerCLIClient.go --output=airflow/mocks --dir=airflow --outpkg=mocks --name=DockerCLIClient
//go:generate go run github.com/vektra/mockery/v2 --filename=Client.go --output=airflow-client/mocks --dir=airflow-client --outpkg=airflow_mocks --name=Client
//go:generate go run github.com/vektra/mockery/v2 --filename=client.go --output=astro-client-core/mocks --dir=astro-client-core --outpkg=astrocore_mocks --name=ClientWithResponsesInterface
//go:generate go run github.com/vektra/mockery/v2 --filename=client.go --output=astro-client-iam-core/mocks --dir=astro-client-iam-core --outpkg=astroiamcore_mocks --name=ClientWithResponsesInterface
//go:generate go run github.com/vektra/mockery/v2 --filename=client.go --output=astro-client-platform-core/mocks --dir=astro-client-platform-core --outpkg=astroplatformcore_mocks --name=ClientWithResponsesInterface
//go:generate go run github.com/vektra/mockery/v2 --filename=Azure.go --output=pkg/azure/mocks --dir=pkg/azure --outpkg=azure_mocks --name=Azure

func main() {
	// TODO: Remove this when version logic is implemented
	fs := afero.NewOsFs()
	config.InitConfig(fs)
	if err := cmd.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}

	// platform specific terminal initialization:
	// this should run for all commands,
	// for most of the architectures there's no requirements:
	ansi.InitConsole()
}
