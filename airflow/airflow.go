package airflow

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/astronomerio/astro-cli/airflow/include"
	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/pkg/fileutil"
	"github.com/astronomerio/astro-cli/pkg/httputil"
	"github.com/astronomerio/astro-cli/config"
)

var (
	http = httputil.NewHTTPClient()
	api = houston.NewHoustonClient(http)
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

	fmt.Println(response.Message)
	fmt.Printf("\nAirflow Dashboard: https://%s-airflow.%s\n", deployment.ReleaseName, config.CFG.CloudDomain.GetString())
	fmt.Printf("Flower Dashboard: https://%s-flower.%s\n", deployment.ReleaseName, config.CFG.CloudDomain.GetString())
	fmt.Printf("Grafana Dashboard: https://%s-grafana.%s\n", deployment.ReleaseName, config.CFG.CloudDomain.GetString())
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
