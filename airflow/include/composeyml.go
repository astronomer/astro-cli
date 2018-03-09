package include

import "strings"

// Composeyml is the docker-compose template
var Composeyml = strings.TrimSpace(`
version: '2'

networks:
	airflow:
	driver: bridge

volumes:
	postgres_data: {}
	airflow_logs: {}

services:
	postgres:
	image: postgres:10.1-alpine
	restart: unless-stopped
	networks:
		- airflow
	labels:
		io.astronomer.docker: "true"
		io.astronomer.docker.cli: "true"
	ports:
		- {{ .PostgresPort }}:{{ .PostgresPort }}
	volumes:
		- postgres_data:/var/lib/postgresql/data
	environment:
		POSTGRES_USER: {{ .PostgresUser }}
		POSTGRES_PASSWORD: {{ .PostgresPassword }}

	scheduler:
	image: {{ .AirflowImage }}
	command: ["airflow", "scheduler"]
	restart: unless-stopped
	networks:
		- airflow
	user: {{ .AirflowUser }}
	labels:
		io.astronomer.docker: "true"
		io.astronomer.docker.cli: "true"
		io.astronomer.docker.component: "airflow-scheduler"
	depends_on:
		- postgres
	environment:
		AIRFLOW__CORE__EXECUTOR: LocalExecutor
		AIRFLOW__CORE__SQL_ALCHEMY_CONN: postgresql://{{ .PostgresUser }}:{{ .PostgresPassword }}@{{ .PostgresHost }}:{{ .PostgresPort }}
	volumes:
		- {{ .AirflowHome }}/dags:/usr/local/airflow/dags:ro
		- {{ .AirflowHome }}/plugins:/usr/local/airflow/plugins:ro
		- {{ .AirflowHome }}/include:/usr/local/airflow/include:ro
		- airflow_logs:/usr/local/airflow/logs

	webserver:
	image: {{ .AirflowImage }}
	command: ["airflow", "webserver"]
	restart: unless-stopped
	networks:
		- airflow
	user: {{ .AirflowUser }}
	labels:
		io.astronomer.docker: "true"
		io.astronomer.docker.cli: "true"
		io.astronomer.docker.component: "airflow-webserver"
	depends_on:
		- scheduler
		- postgres
	environment:
		AIRFLOW__CORE__EXECUTOR: LocalExecutor
		AIRFLOW__CORE__SQL_ALCHEMY_CONN: postgresql://{{ .PostgresUser }}:{{ .PostgresPassword }}@{{ .PostgresHost }}:{{ .PostgresPort }}
	ports:
		- {{ .AirflowWebserverPort }}:{{ .AirflowWebserverPort }}
	volumes:
		- {{ .AirflowHome }}/dags:/usr/local/airflow/dags:ro
		- {{ .AirflowHome }}/plugins:/usr/local/airflow/plugins:ro
		- {{ .AirflowHome }}/include:/usr/local/airflow/include:ro
		- airflow_logs:/usr/local/airflow/logs
`)
