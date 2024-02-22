package main

import (
	"os"

	"github.com/astronomer/astro-cli/cmd"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/spf13/afero"
)

//go:generate go run github.com/deepmap/oapi-codegen/cmd/oapi-codegen --version
//go:generate go run github.com/deepmap/oapi-codegen/cmd/oapi-codegen --config ./astro-client-core/api.cfg.yaml ../astro/apps/core/docs/public/v1alpha1/public_v1alpha1.yaml
//go:generate go run github.com/deepmap/oapi-codegen/cmd/oapi-codegen --config ./astro-client-iam-core/api.cfg.yaml ../astro/apps/core/docs/iam/v1beta1/iam_v1beta1.yaml
//go:generate go run github.com/deepmap/oapi-codegen/cmd/oapi-codegen --config ./astro-client-platform-core/api.cfg.yaml ../astro/apps/core/docs/platform/v1beta1/platform_v1beta1.yaml

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
