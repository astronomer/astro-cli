package airflow

const (
	QuayBaseImageName               = "quay.io/astronomer"
	AstroImageRegistryBaseImageName = "astrocrpublic.azurecr.io"
)

// DefaultTestPath is the DAG integrity test a deploy's parse step runs.
const DefaultTestPath = ".astro/test_dag_integrity_default.py"
