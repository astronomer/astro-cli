GIT_COMMIT_SHORT=$(shell git rev-parse --short HEAD)
VERSION ?= SNAPSHOT-${GIT_COMMIT_SHORT}

LDFLAGS_VERSION=-X github.com/astronomer/astro-cli/version.CurrVersion=${VERSION}
ENVTEST_ASSETS_DIR=$(shell pwd)/testbin

OUTPUT ?= astro
# golangci-lint version
GOLANGCI_LINT_VERSION ?=v1.50.1

lint:
	@test -f ${ENVTEST_ASSETS_DIR}/golangci-lint || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ${ENVTEST_ASSETS_DIR} ${GOLANGCI_LINT_VERSION}
	${ENVTEST_ASSETS_DIR}/golangci-lint version
	${ENVTEST_ASSETS_DIR}/golangci-lint run --timeout 3m0s

build:
	go build -o ${OUTPUT} -ldflags "${LDFLAGS_VERSION}" main.go

core_api_gen:
	oapi-codegen -include-tags=User,Organization,Invite -generate=types,client -package=astrocore ../astro/apps/core/docs/public/public_v1alpha1.yaml > ./astro-client-core/api.gen.go

test:
	go test -count=1 -cover ./...
	go test -coverprofile=coverage.txt -covermode=atomic ./...

mock: mock_airflow mock_houston mock_astro mock_pkg mock_sql_cli

mock_houston:
	mockery --filename=ClientInterface.go --output=houston/mocks --dir=houston --outpkg=houston_mocks --name ClientInterface

mock_airflow:
	mockery --filename=RegistryHandler.go --output=airflow/mocks --dir=airflow --outpkg=mocks --name RegistryHandler
	mockery --filename=ImageHandler.go --output=airflow/mocks --dir=airflow --outpkg=mocks --name ImageHandler
	mockery --filename=ContainerHandler.go --output=airflow/mocks --dir=airflow --outpkg=mocks --name ContainerHandler
	mockery --filename=DockerComposeAPI.go --output=airflow/mocks --dir=airflow --outpkg=mocks --name DockerComposeAPI
	mockery --filename=DockerRegistryAPI.go --output=airflow/mocks --dir=airflow --outpkg=mocks --name DockerRegistryAPI
	mockery --filename=DockerCLIClient.go --output=airflow/mocks --dir=airflow --outpkg=mocks --name DockerCLIClient

mock_astro:
	mockery --filename=Client.go --output=astro-client/mocks --dir=astro-client --outpkg=astro_mocks --name Client

mock_astro_core:
	mockery --filename=client.go --output=astro-client-core/mocks --dir=astro-client-core --outpkg=astrocore_mocks --name ClientWithResponsesInterface

mock_pkg:
	mockery --filename=Azure.go --output=pkg/azure/mocks --dir=pkg/azure --outpkg=azure_mocks --name Azure

mock_sql_cli:
	mockery --filename="flow.go" --output="sql/mocks" --dir=sql/ --outpkg=mocks --name DockerBind

codecov:
	@eval $$(curl -s https://codecov.io/bash)
