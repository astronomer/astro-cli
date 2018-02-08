package airflow

import (
	"fmt"

	"github.com/astronomerio/astro-cli/docker"
)

// Create initializes and creates a new airflow project
// TODO: Scaffold Dockerfile, requirements.txt, packages.txt, .astro/config
func Create() {
}

// Build builds the airflow project
func Build(name, tag string) {
	image := imageName(name, tag)
	docker.Exec("build", "-t", image, ".")
}

// Deploy pushes a new docker image
// TODO: Check for uncommitted git changes
// TODO: Command to bump version or create version automatically
func Deploy(name, tag string) {
	image := imageName(name, tag)
	remoteImage := fmt.Sprintf("%s/%s", docker.CloudRegistry, image)
	docker.Exec("tag", image, remoteImage)
	docker.Exec("push", remoteImage)
}

func imageName(name, tag string) string {
	return fmt.Sprintf("%s/%s:%s", name, "airflow", tag)
}
