package airflow

import (
	_ "embed"
	"fmt"
)

// Embedded templates consumed within this repo. Project scaffolding
// (gitignore, example DAG, etc.) lives in pkg/scaffold — do not duplicate it
// here.
var (
	//go:embed include/airflow2/astronomermonitoringdag.py
	Af2MonitoringDag string

	//go:embed include/airflow3/astronomermonitoringdag.py
	Af3MonitoringDag string
)

// repositoryName creates an airflow repository name
func repositoryName(name string) string {
	return fmt.Sprintf("%s/%s", name, componentName)
}

// imageName creates an airflow image name
func ImageName(name, tag string) string {
	return fmt.Sprintf("%s:%s", repositoryName(name), tag)
}
