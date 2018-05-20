package airflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/iancoleman/strcase"

	"github.com/astronomerio/astro-cli/airflow/include"
	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/pkg/fileutil"
	"github.com/astronomerio/astro-cli/pkg/httputil"
)

var (
	http = httputil.NewHTTPClient()
	api  = houston.NewHoustonClient(http)
)

func initDirs(root string, dirs []string) bool {
	// Any inputs exist
	exists := false

	// Create the dirs
	for _, dir := range dirs {
		// Create full path to directory
		fullpath := filepath.Join(root, dir)

		// Move on if already exists
		if fileutil.Exists(fullpath) {
			exists = true
			continue
		}

		// Create directory
		if err := os.MkdirAll(dir, 0777); err != nil {
			fmt.Println(err)
		}
	}

	return exists
}

func initFiles(root string, files map[string]string) bool {
	// Any inputs exist
	exists := false

	// Create the files
	for file, content := range files {
		// Create full path to file
		fullpath := filepath.Join(root, file)

		// Move on if already exiss
		if fileutil.Exists(fullpath) {
			exists = true
			continue
		}

		// Write files out
		if err := fileutil.WriteStringToFile(fullpath, content); err != nil {
			fmt.Println(err)
		}
	}

	return exists
}

// Init will scaffold out a new airflow project
func Init(path string) error {
	// List of directories to create
	dirs := []string{"dags", "plugins", "include"}

	// Map of files to create
	files := map[string]string{
		".dockerignore":       include.Dockerignore,
		"Dockerfile":          include.Dockerfile,
		"packages.txt":        "",
		"requirements.txt":    "",
		"dags/example-dag.py": include.Exampledag,
	}

	// Initailize directories
	initDirs(path, dirs)

	// Initialize files
	initFiles(path, files)

	return nil
}

// Create new airflow deployment
func Create(title string) error {
	response, err := api.CreateDeployment(title)
	if err != nil {
		return err
	}
	deployment, err := api.FetchDeployment(response.Id)
	if err != nil {
		return err
	}

	fmt.Println(response.Message)

	if response.Success {
		fmt.Printf("\nAirflow Dashboard: https://%s-airflow.%s\n", deployment.ReleaseName, config.CFG.CloudDomain.GetString())
		fmt.Printf("Flower Dashboard: https://%s-flower.%s\n", deployment.ReleaseName, config.CFG.CloudDomain.GetString())
		fmt.Printf("Grafana Dashboard: https://%s-grafana.%s\n", deployment.ReleaseName, config.CFG.CloudDomain.GetString())
	}

	return nil
}

// List all airflow deployments
func List() error {
	deployments, err := api.FetchDeployments()
	if err != nil {
		return err
	}

	for _, d := range deployments {
		rowTmp := "Title: %s\nId: %s\nRelease: %s\nVersion: %s\n\n"
		fmt.Printf(rowTmp, d.Title, d.Id, d.ReleaseName, d.Version)
	}
	return nil
}

func validateOrCreateProjectName(path, projectName string) (string, error) {
	if len(projectName) != 0 {
		projectNameValid := regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9_-]*[A-Za-z0-9])?$`).MatchString
		if !projectNameValid(projectName) {
			return "", errors.New("Project name is invalid")
		}
	} else {
		projectDirectory := filepath.Base(path)

		return strings.Replace(strcase.ToSnake(projectDirectory), "_", "-", -1), nil
	}

	return projectName, nil
}
