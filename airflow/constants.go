package airflow

import "fmt"

const (
	QuayBaseImageName               = "quay.io/astronomer"
	AstroImageRegistryBaseImageName = "astrocrpublic.azurecr.io"
	AstronomerCertifiedImageName    = "ap-airflow"
	AstroRuntimeAirflow2ImageName   = "astro-runtime"
	AstroRuntimeAirflow3ImageName   = "runtime"

	airflowVersionLabelName = "io.astronomer.docker.airflow.version"
	runtimeVersionLabelName = "io.astronomer.docker.runtime.version"

	warningTriggererDisabledNoVersionDetectedMsg = "warning: could not find the version of Airflow or Runtime you're using, are you using an official Docker image for your project ? Disabling Airflow triggerer."

	WebserverDockerContainerName    = "webserver"
	SchedulerDockerContainerName    = "scheduler"
	TriggererDockerContainerName    = "triggerer"
	APIServerDockerContainerName    = "api-server"
	PostgresDockerContainerName     = "postgres"
	DAGProcessorDockerContainerName = "dag-processor"
)

var (
	// Pytest constants
	DefaultTestPath                   = ".astro/test_dag_integrity_default.py"
	FullAstronomerCertifiedImageName  = fmt.Sprintf("%s/%s", QuayBaseImageName, AstronomerCertifiedImageName)
	FullAstroRuntimeAirflow2ImageName = fmt.Sprintf("%s/%s", QuayBaseImageName, AstroRuntimeAirflow2ImageName)
	FullAstroRuntimeAirflow3ImageName = fmt.Sprintf("%s/%s", AstroImageRegistryBaseImageName, AstroRuntimeAirflow3ImageName)
)
