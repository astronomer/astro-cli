package utils

import (
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/pkg/util"
)

// AddBuildSecretFlag registers --build-secret, which collects one docker build
// --secret spec per repetition into target.
func AddBuildSecretFlag(flags *pflag.FlagSet, target *[]string) {
	flags.StringArrayVar(target, "build-secret", []string{}, util.BuildSecretUsage)
}
