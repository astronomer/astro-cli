package airflow

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	airflowTypes "github.com/astronomer/astro-cli/airflow/types"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/util"
)

const (
	RuntimeImageLabel = "io.astronomer.docker.runtime.version"
	pytestDirectory   = "tests"
	componentName     = "airflow"
)

// DAGChecker runs a project's DAG checks in its image: the parse and
// pytest steps of a 1.x project's deploy. It is what is left of the 1.x
// `astro dev` container handler, which drove docker compose.
type DAGChecker struct {
	airflowHome  string
	envFile      string
	dockerfile   string
	imageHandler ImageHandler
}

func NewDAGChecker(airflowHome, envFile, dockerfile, imageName string) (*DAGChecker, error) {
	if imageName == "" {
		// Get project name from config
		projectName, err := ProjectNameUnique()
		if err != nil {
			return nil, fmt.Errorf("error retrieving working directory: %w", err)
		}
		imageName = projectName
	}

	return &DAGChecker{
		airflowHome:  airflowHome,
		envFile:      envFile,
		dockerfile:   dockerfile,
		imageHandler: DockerImageInit(ImageName(imageName, "latest")),
	}, nil
}

// Pytest creates and runs a container containing the users airflow image, requirments, packages, and volumes(DAGs folder, etc...)
// These containers runs pytest on a specified pytest file (pytestFile). A deploy's --pytest and --parse use it
func (d *DAGChecker) Pytest(pytestFile, customImageName, deployImageName, pytestArgsString string, buildSecrets []string) (string, error) {
	// deployImageName may be provided to the function if it is being used in the deploy command
	if deployImageName == "" {
		// build image
		if customImageName == "" {
			err := d.imageHandler.Build(d.dockerfile, buildSecrets, airflowTypes.ImageBuildConfig{Path: d.airflowHome})
			if err != nil {
				return "", err
			}
		} else {
			// skip build if an customImageName is passed
			err := d.imageHandler.TagLocalImage(customImageName)
			if err != nil {
				return "", err
			}
		}
	}

	// determine pytest args and file
	pytestArgs := strings.Fields(pytestArgsString)

	// Determine pytest file
	if pytestFile != DefaultTestPath {
		if !strings.Contains(pytestFile, pytestDirectory) {
			pytestFile = pytestDirectory + "/" + pytestFile
		} else if pytestFile == "" {
			pytestFile = pytestDirectory + "/"
		}
	}

	// run pytests
	exitCode, err := d.imageHandler.Pytest(pytestFile, d.airflowHome, d.envFile, "", pytestArgs, false, airflowTypes.ImageBuildConfig{Path: d.airflowHome})
	if err != nil {
		return exitCode, err
	}
	if code, convErr := strconv.Atoi(exitCode); convErr == nil && code == 0 { // exit code 0 means the pytests passed
		return "", nil
	}
	return exitCode, errors.New("something went wrong while Pytesting your Dags")
}

func (d *DAGChecker) Parse(customImageName, deployImageName string, buildSecrets []string) error {
	// check for file
	path := d.airflowHome + "/" + DefaultTestPath

	fileExist, err := util.Exists(path)
	if err != nil {
		return err
	}
	if !fileExist {
		// Only a 1.x project deploys with --parse, and Astro CLI 1.x's
		// `astro dev init` wrote this file into it; v2 writes it nowhere. A
		// project without it has never had a parse check to run, so the deploy
		// goes on, as it always has, but says so.
		fmt.Println("\nSkipping the DAG parse check: it runs " + path + ", which this project does not have. " +
			"Astro CLI 1.x's astro dev init created that file; add it back to the project to run the check.")

		return nil
	}

	fmt.Println("Checking your Dags for errors…")

	pytestFile := DefaultTestPath
	exitCode, err := d.Pytest(pytestFile, customImageName, deployImageName, "", buildSecrets)
	if err != nil {
		if code, convErr := strconv.Atoi(exitCode); convErr == nil && code == 1 { // exit code 1 means tests failed
			return errors.New("See above for errors detected in your Dags")
		}
		return errors.Wrap(err, "something went wrong while parsing your Dags")
	}
	fmt.Println(ansi.Green("✔") + " No errors detected in your Dags ")
	return err
}
