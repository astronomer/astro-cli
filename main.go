package main

import (
	"os"

	"github.com/spf13/afero"

	"github.com/astronomer/astro-cli/cmd"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/ansi"
)

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --version
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config ./astro-client-v1/api.cfg.yaml ../astro/apps/core/docs/versioned/v1.0/versionedapi_v1.0.yaml
//go:generate go run ./scripts/patch-v1-gen ./astro-client-v1/api.gen.go
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config ./astro-client-v1alpha1/api.cfg.yaml ../astro/apps/core/docs/public/v1alpha1/public_v1alpha1.yaml

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
