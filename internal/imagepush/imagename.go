package imagepush

import "fmt"

const componentName = "airflow"

// repositoryName creates an airflow repository name
func repositoryName(name string) string {
	return fmt.Sprintf("%s/%s", name, componentName)
}

// ImageName creates an airflow image name
func ImageName(name, tag string) string {
	return fmt.Sprintf("%s:%s", repositoryName(name), tag)
}
