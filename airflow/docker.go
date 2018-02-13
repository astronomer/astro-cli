package airflow

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	"github.com/astronomerio/astro-cli/config"
	"github.com/docker/libcompose/docker"
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

// Start starts a local airflow development cluster
func Start(path string) error {
	// Infer the project name using directory
	proj := projectName(path)

	// Generate the docker-compose yaml
	yaml := generateConfig(proj)

	// Create the project
	project, err := docker.NewProject(&ctx.Context{
		Context: project.Context{
			ComposeBytes: [][]byte{[]byte(yaml)},
			ProjectName:  proj,
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
	// Infer the project name using directory
	proj := projectName(path)

	// Generate the docker-compose yaml
	yaml := generateConfig(proj)

	// Create the project
	project, err := docker.NewProject(&ctx.Context{
		Context: project.Context{
			ComposeBytes: [][]byte{[]byte(yaml)},
			ProjectName:  proj,
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

// generateConfig generates the docker-compose config
func generateConfig(projectName string) string {
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
		AirflowImage:         fmt.Sprintf("%s/airflow", projectName),
		AirflowHome:          "/home/schnie/repos/open-example-dags",
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

// projectName converts a path to project name
func projectName(path string) string {
	return filepath.Base(path)
}
