package local

import (
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/util"
)

func addBuildSecretFlag(cmd *cobra.Command, target *[]string) {
	cmd.Flags().StringArrayVar(target, "build-secret", nil, util.BuildSecretUsage)
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

// warnMissingBuildSecrets runs before a Docker-mode start builds the project's
// own Dockerfile. It refuses a build secret whose variable is unset, and warns
// about each secret the file mounts that no build secret supplies, returning
// those for the start to name again if the build fails.
func warnMissingBuildSecrets(r Renderer, p localrt.Plan) (util.MissingSecrets, error) {
	if p.Mode != localrt.ModeDocker || p.Dockerfile == "" {
		return util.MissingSecrets{}, nil
	}
	missing, err := util.CheckBuildSecrets(p.ProjectPath, p.Dockerfile, p.BuildSecrets)
	for _, w := range missing.Warnings() {
		emitWarning(r, event{Event: "warning", Text: w})
	}
	return missing, err
}
