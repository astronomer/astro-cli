GIT_COMMIT_SHORT=$(shell git rev-parse --short HEAD)
VERSION ?= SNAPSHOT-${GIT_COMMIT_SHORT}

LDFLAGS_VERSION=-X github.com/astronomer/astro-cli/version.CurrVersion=${VERSION}

CORE_OPENAPI_SPEC=../astro/apps/core/docs/public/v1alpha1/public_v1alpha1.yaml
CORE_IAM_OPENAPI_SPEC=../astro/apps/core/docs/iam/v1beta1/iam_v1beta1.yaml
CORE_PLATFORM_OPENAPI_SPEC=../astro/apps/core/docs/platform/v1beta1/platform_v1beta1.yaml

OUTPUT ?= astro
# golangci-lint version
GOLANGCI_LINT_VERSION ?=v1.50.1

PWD=$(shell pwd)

## Location to install dependencies to
ENVTEST_ASSETS_DIR=$(shell pwd)/testbin
$(ENVTEST_ASSETS_DIR):
	mkdir -p $(ENVTEST_ASSETS_DIR)

## Tool Binaries
MOCKERY ?= $(ENVTEST_ASSETS_DIR)/mockery

## Tool versions
MOCKERY_VERSION ?= v2.32.0

.PHONY: kustomize
mockery: $(ENVTEST_ASSETS_DIR)
	(test -s $(MOCKERY) && $(MOCKERY) --version | grep -i $(MOCKERY_VERSION)) || GOBIN=$(ENVTEST_ASSETS_DIR) go install github.com/vektra/mockery/v2@$(MOCKERY_VERSION)


lint:
	@test -f ${ENVTEST_ASSETS_DIR}/golangci-lint || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ${ENVTEST_ASSETS_DIR} ${GOLANGCI_LINT_VERSION}
	${ENVTEST_ASSETS_DIR}/golangci-lint version
	${ENVTEST_ASSETS_DIR}/golangci-lint run --timeout 15m0s

build:
	go build -o ${OUTPUT} -ldflags "${LDFLAGS_VERSION}" main.go

core_api_gen:
    ifeq (, $(shell which oapi-codegen))
	go install github.com/deepmap/oapi-codegen/cmd/oapi-codegen@latest
    endif
	oapi-codegen -include-tags=User,Organization,Invite,Workspace,Cluster,Options,Team,ApiToken,Deployment,Deploy,Environment -generate=types,client -package=astrocore "${CORE_OPENAPI_SPEC}" > ./astro-client-core/api.gen.go
	make mock_astro_core

	oapi-codegen -include-tags=User,Invite,Team,ApiToken -generate=types,client -package=astroiamcore "${CORE_IAM_OPENAPI_SPEC}" > ./astro-client-iam-core/api.gen.go
	make mock_astro_iam_core

	oapi-codegen -include-tags=Organization,Workspace,Cluster,Options,Deployment -generate=types,client -package=astroplatformcore "${CORE_PLATFORM_OPENAPI_SPEC}" > ./astro-client-platform-core/api.gen.go
	make mock_astro_platform_core

test:
	go test -count=1 -cover -coverprofile=coverage.txt -covermode=atomic ./...

temp-astro:
	cd $(shell mktemp -d) && ${PWD}/astro dev init

mock: mockery mock_airflow mock_houston mock_astro mock_pkg mock_astro_core mock_airflow_api

mock_houston:
	$(MOCKERY) --filename=ClientInterface.go --output=houston/mocks --dir=houston --outpkg=houston_mocks --name ClientInterface

mock_airflow:
	$(MOCKERY) --filename=RegistryHandler.go --output=airflow/mocks --dir=airflow --outpkg=mocks --name RegistryHandler
	$(MOCKERY) --filename=ImageHandler.go --output=airflow/mocks --dir=airflow --outpkg=mocks --name ImageHandler
	$(MOCKERY) --filename=ContainerHandler.go --output=airflow/mocks --dir=airflow --outpkg=mocks --name ContainerHandler
	$(MOCKERY) --filename=DockerComposeAPI.go --output=airflow/mocks --dir=airflow --outpkg=mocks --name DockerComposeAPI
	$(MOCKERY) --filename=DockerRegistryAPI.go --output=airflow/mocks --dir=airflow --outpkg=mocks --name DockerRegistryAPI
	$(MOCKERY) --filename=DockerCLIClient.go --output=airflow/mocks --dir=airflow --outpkg=mocks --name DockerCLIClient

mock_airflow_api:
	$(MOCKERY) --filename=Client.go --output=airflow-client/mocks --dir=airflow-client --outpkg=airflow_mocks --name Client

mock_astro:
	$(MOCKERY) --filename=Client.go --output=astro-client/mocks --dir=astro-client --outpkg=astro_mocks --name Client

mock_astro_core:
	$(MOCKERY) --filename=client.go --output=astro-client-core/mocks --dir=astro-client-core --outpkg=astrocore_mocks --name ClientWithResponsesInterface

mock_astro_iam_core:
	$(MOCKERY) --filename=client.go --output=astro-client-iam-core/mocks --dir=astro-client-iam-core --outpkg=astroiamcore_mocks --name ClientWithResponsesInterface

mock_astro_platform_core:
	$(MOCKERY) --filename=client.go --output=astro-client-platform-core/mocks --dir=astro-client-platform-core --outpkg=astroplatformcore_mocks --name ClientWithResponsesInterface

mock_pkg:
	$(MOCKERY) --filename=Azure.go --output=pkg/azure/mocks --dir=pkg/azure --outpkg=azure_mocks --name Azure

codecov:
	@eval $$(curl -s https://codecov.io/bash)
