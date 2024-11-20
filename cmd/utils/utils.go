package utils

import (
	"runtime"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func EnsureProjectDir(cmd *cobra.Command, args []string) error {
	isProjectDir, err := config.IsProjectDir(config.WorkingPath)
	if err != nil {
		return errors.Wrap(err, ansi.Red("failed to verify that your working directory is an Astro project.\nTry running astro dev init to turn your working directory into an Astro project"))
	}

	if !isProjectDir {
		return errors.New(ansi.Red("this is not an Astro project directory.\nChange to another directory or run astro dev init to turn your working directory into an Astro project\n"))
	}

	return nil
}

func GetDefaultDeployDescription(isDagOnlyDeploy bool) string {
	if isDagOnlyDeploy {
		return "Deployed via <astro deploy --dags>"
	}

	return "Deployed via <astro deploy>"
}

func IsMac() bool {
	return runtime.GOOS == "darwin"
}
