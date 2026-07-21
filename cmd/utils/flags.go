package utils

import (
	"github.com/spf13/pflag"
)

// AddBuildSecretFlags registers --build-secret together and its deprecated
// alias --build-secrets. Both register args to the same target slice.
func AddBuildSecretFlags(flags *pflag.FlagSet, target *[]string) {
	flags.StringArrayVar(target, "build-secret", []string{}, "Secret to expose to the build. See https://docs.docker.com/build/building/secrets/. Repeat to specify multiple secrets. (format: \"id=mysecret[,src=/local/secret]\")")
	flags.Var(flags.Lookup("build-secret").Value, "build-secrets", "Deprecated: use --build-secret instead")
	flags.MarkDeprecated("build-secrets", "use --build-secret instead") //nolint:errcheck
}
