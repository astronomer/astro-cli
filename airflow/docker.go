package airflow

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"os"

	"github.com/astronomerio/astro-cli/config"
	docker "github.com/astronomerio/astro-cli/docker"
	dockercompose "github.com/docker/libcompose/docker"
	"github.com/docker/libcompose/docker/ctx"
	"github.com/docker/libcompose/project"
	"github.com/docker/libcompose/project/options"
)

// ComposeConfig is input data to docker compose yaml template
type ComposeConfig struct {
	PostgresUser         string
	PostgresPassword     string
	PostgresHost         string
	PostgresPort         string
	AirflowImage         string
	AirflowHome          string
	AirflowUser          string
	AirflowWebserverPort string
}

// imageName creates an airflow image name
func imageName(name, tag string) string {
	return fmt.Sprintf("%s/%s:%s", name, "ap-airflow", tag)
}

// imageBuild builds the airflow project
func imageBuild(path, imageName string) {
	// Change to location of Dockerfile
	os.Chdir(path)

	fmt.Printf("Building %s...\n", imageName)
	docker.Exec("build", "-t", imageName, ".")
}

// generateConfig generates the docker-compose config
func generateConfig(projectName, airflowHome string) string {
	tmpl, err := template.New("yml").Parse(composeyml)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	config := ComposeConfig{
		PostgresUser:         config.GetString(config.CFGPostgresUser),
		PostgresPassword:     config.GetString(config.CFGPostgresPassword),
		PostgresHost:         config.GetString(config.CFGPostgresHost),
		PostgresPort:         config.GetString(config.CFGPostgresPort),
		AirflowImage:         imageName(projectName, "latest"),
		AirflowHome:          airflowHome,
		AirflowUser:          "astro",
		AirflowWebserverPort: "8080",
	}

	buff := new(bytes.Buffer)
	err = tmpl.Execute(buff, config)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	return buff.String()
}

// Start starts a local airflow development cluster
func Start(path string) error {
	// Get project name from config
	projectName := config.GetString(config.CFGProjectName)

	// Build this project image
	imageBuild(path, imageName(projectName, "latest"))

	// Generate the docker-compose yaml
	yaml := generateConfig(projectName, path)

	// Create the project
	project, err := dockercompose.NewProject(&ctx.Context{
		Context: project.Context{
			ComposeBytes: [][]byte{[]byte(yaml)},
			ProjectName:  projectName,
		},
	}, nil)

	if err != nil {
		fmt.Println(err)
	}

	// Start up our project
	err = project.Up(context.Background(), options.Up{})

	if err != nil {
		fmt.Println(err)
	}

	return nil
}

// Stop stops a local airflow development cluster
func Stop(path string) error {
	// Get project name from config
	projectName := config.GetString(config.CFGProjectName)

	// Generate the docker-compose yaml
	yaml := generateConfig(projectName, path)

	// Create the project
	project, err := dockercompose.NewProject(&ctx.Context{
		Context: project.Context{
			ComposeBytes: [][]byte{[]byte(yaml)},
			ProjectName:  projectName,
		},
	}, nil)

	if err != nil {
		fmt.Println(err)
	}

	// Shut down our project
	err = project.Down(context.Background(), options.Down{RemoveVolume: true})

	if err != nil {
		fmt.Println(err)
	}

	return nil
}

// Deploy pushes a new docker image
// TODO: Check for uncommitted git changes
// TODO: Command to bump version or create version automatically
func Deploy(path, name, tag string) error {
	imageName := imageName(name, tag)
	imageBuild(path, imageName)
	fmt.Printf("Pushing %s...\n", imageName)
	remoteImage := fmt.Sprintf("%s/%s", docker.CloudRegistry, imageName)
	docker.Exec("tag", imageName, remoteImage)
	docker.Exec("push", remoteImage)
	return nil
}

// PS prints the running airflow containers
func PS() error {
	return nil
}
