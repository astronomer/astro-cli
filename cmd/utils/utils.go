package utils

import (
	"github.com/astronomer/astro-cli/config"
	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func EnsureProjectDir(cmd *cobra.Command, args []string) error {
	isProjectDir, err := config.IsProjectDir(config.WorkingPath)
	if err != nil {
		return errors.Wrap(err, "failed to verify that your working directory is an Astro project.\nTry running astro dev init to turn your working directory into an Astro project")
	}

	if !isProjectDir {
		return errors.New("this is not an Astro project directory.\nChange to another directory or run astro dev init to turn your working directory into an Astro project")
	}

	return nil
}

func IsRequiredVersionMet(currentVersion, requiredVersion string) (bool, error) {
	v1, err := version.NewVersion(currentVersion)
	if err != nil {
		return false, err
	}
	constraints, err := version.NewConstraint(requiredVersion)
	if err != nil {
		return false, err
	}
	if constraints.Check(v1) {
		return true, nil
	}
	return false, nil
}
