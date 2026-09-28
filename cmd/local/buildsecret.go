package local

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/util"
)

func addBuildSecretFlag(cmd *cobra.Command, target *[]string) {
	cmd.Flags().StringArrayVar(target, "build-secret", nil, util.BuildSecretUsage)
}

func resolveBuildSecrets(flag []string) []string {
	return util.ResolveBuildSecrets(flag, os.Getenv(util.BuildSecretInputEnv))
}

// checkBuildSecrets holds a --build-secret to what `astro deploy` holds it to.
// Only the flag is checked, not BUILD_SECRET_INPUT: a variable a CI runner
// exports for every job must not fail a project that builds no Dockerfile.
func checkBuildSecrets(r Renderer, flag []string, p localrt.Plan) error {
	if len(flag) == 0 {
		return nil
	}
	if p.Dockerfile == "" {
		return util.ErrBuildSecretNeedsDockerfile
	}
	if p.Mode != localrt.ModeDocker {
		emitWarning(r, event{Event: "warning", Text: "standalone mode builds no image, so --build-secret is not used. Run in Docker mode (--docker) to build with it"})
	}
	return nil
}
