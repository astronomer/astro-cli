package cmd

import (
	"errors"
)

var (
	errInvalidBothAirflowAndRuntimeVersions        = errors.New("You provided both a runtime version and an Airflow version. You have to provide only one of these to initialize your project.") //nolint
	errInvalidBothAirflowAndRuntimeVersionsUpgrade = errors.New("You provided both a runtime version and an Airflow version. You have to provide only one of these to upgrade.")                 //nolint
	errInvalidBothDeploymentIDandCustomImage       = errors.New("You provided both a Deployment ID and a Custom image. You have to provide only one of these to upgrade.")                       //nolint
	errInvalidBothDeploymentIDandVersion           = errors.New("You provided both a Deployment ID and a version. You have to provide only one of these to upgrade.")                            //nolint
	errInvalidBothCustomImageandVersion            = errors.New("You provided both a Custom image and a version. You have to provide only one of these to upgrade.")                             //nolint

	errConfigProjectName = errors.New("project name is invalid")
	errProjectNameSpaces = errors.New("this project name is invalid, a project name cannot contain spaces. Try using '-' instead")

	errInvalidSetArgs    = errors.New("must specify exactly two arguments (key value) when setting a config")
	errInvalidConfigPath = errors.New("config does not exist, check your config key")
)
