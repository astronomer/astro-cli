package cmd

import (
	"errors"
)

var (
	errInvalidBothAirflowAndRuntimeVersions        = errors.New("you provided both a runtime version and an Airflow version. You have to provide only one of these to initialize your project") //nolint
	errInvalidBothAirflowAndRuntimeVersionsUpgrade = errors.New("you provided both a runtime version and an Airflow version. You have to provide only one of these to upgrade")                 //nolint
	errInvalidBothCustomImageandVersion            = errors.New("you provided both a Custom image and a version. You have to provide only one of these to upgrade")                             //nolint

	errConfigProjectName = errors.New("project name is invalid")

	errInvalidSetArgs    = errors.New("must specify exactly two arguments (key value) when setting a config")
	errInvalidConfigPath = errors.New("config does not exist, check your config key")
)
