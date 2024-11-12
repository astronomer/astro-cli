package utils

import (
	"github.com/astronomer/astro-cli/airflow"
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

func EnsurePreFlightSetup(cmd *cobra.Command, args []string) error {
	if err := EnsureProjectDir(cmd, args); err != nil {
		return err
	}

	if config.CFG.DockerCommand.GetString() == "podman" {
		if err := airflow.InitPodmanMachineCMD(); err != nil {
			return err
		}
	}

	return nil
}

func AirflowStopPreRun(cmd *cobra.Command, args []string) error {
	if err := EnsureProjectDir(cmd, args); err != nil {
		return err
	}

	if config.CFG.DockerCommand.GetString() == "podman" {
		if err := airflow.SetPodmanDockerHost(); err != nil {
			return err
		}
	}

	return nil
}

func AirflowKillPostRun(cmd *cobra.Command, args []string) error {
	if config.CFG.DockerCommand.GetString() == "podman" {
		if err := airflow.StopAndKillPodmanMachine(); err != nil {
			return err
		}
	}
	return nil
}

func GetDefaultDeployDescription(isDagOnlyDeploy bool) string {
	if isDagOnlyDeploy {
		return "Deployed via <astro deploy --dags>"
	}

	return "Deployed via <astro deploy>"
}
