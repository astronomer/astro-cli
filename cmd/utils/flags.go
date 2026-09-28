package utils

import (
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/pkg/util"
)

// AddBuildSecretFlags registers --build-secret together and its deprecated
// alias --build-secrets. Both register args to the same target slice.
func AddBuildSecretFlags(flags *pflag.FlagSet, target *[]string) {
	flags.StringArrayVar(target, "build-secret", []string{}, util.BuildSecretUsage)
	flags.Var(flags.Lookup("build-secret").Value, "build-secrets", "Deprecated: use --build-secret instead")
	flags.MarkDeprecated("build-secrets", "use --build-secret instead") //nolint:errcheck // deprecation marking cannot fail for a registered flag
}
