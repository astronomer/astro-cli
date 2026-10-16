package deploy

import (
	_ "embed"
)

// The monitoring DAG uploadDags adds to a DAG upload, one per Airflow major
// version.
var (
	//go:embed include/airflow2/astronomermonitoringdag.py
	af2MonitoringDag string

	//go:embed include/airflow3/astronomermonitoringdag.py
	af3MonitoringDag string
)
