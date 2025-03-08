package airflow

import "fmt"

const (
	BaseImageName                = "quay.io/astronomer"
	AstronomerCertifiedImageName = "ap-airflow"
	AstroRuntimeImageName        = "astro-runtime"

	airflowVersionLabelName = "io.astronomer.docker.airflow.version"
	runtimeVersionLabelName = "io.astronomer.docker.runtime.version"

	warningTriggererDisabledNoVersionDetectedMsg = "warning: could not find the version of Airflow or Runtime you're using, are you using an official Docker image for your project ? Disabling Airflow triggerer."

	WebserverDockerContainerName    = "webserver"
	SchedulerDockerContainerName    = "scheduler"
	TriggererDockerContainerName    = "triggerer"
	DagProcessorDockerContainerName = "dag-processor"
	PostgresDockerContainerName     = "postgres"
)

var (
	// Pytest constants

	DefaultTestPath                  = ".astro/test_dag_integrity_default.py"
	FullAstronomerCertifiedImageName = fmt.Sprintf("%s/%s", BaseImageName, AstronomerCertifiedImageName)
	FullAstroRuntimeImageName        = fmt.Sprintf("%s/%s", BaseImageName, AstroRuntimeImageName)
)
