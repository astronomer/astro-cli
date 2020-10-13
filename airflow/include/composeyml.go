package include

import "strings"

// Composeyml is the docker-compose template
var Composeyml = strings.TrimSpace(`
version: '2'

networks:
  airflow:
    driver: bridge

volumes:
  postgres_data:
    driver: local
  airflow_logs:
    driver: local

services:
  postgres:
    image: postgres:12.2
    restart: unless-stopped
    networks:
      - airflow
    labels:
      io.astronomer.docker: "true"
      io.astronomer.docker.cli: "true"
    ports:
      - {{ .PostgresPort }}:5432
    volumes:
      - postgres_data:/var/lib/postgresql/data
    environment:
      POSTGRES_USER: {{ .PostgresUser }}
      POSTGRES_PASSWORD: {{ .PostgresPassword }}

  scheduler:
    image: {{ .AirflowImage }}
    command: >
      bash -c "(airflow upgradedb || airflow db upgrade) && airflow scheduler"
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
      AIRFLOW__CORE__SQL_ALCHEMY_CONN: postgresql://{{ .PostgresUser }}:{{ .PostgresPassword }}@{{ .PostgresHost }}:5432
      AIRFLOW__CORE__LOAD_EXAMPLES: "False"
      AIRFLOW__CORE__FERNET_KEY: "d6Vefz3G9U_ynXB3cr7y_Ak35tAHkEGAVxuz_B-jzWw="
    volumes:
      - {{ .AirflowHome }}/dags:/usr/local/airflow/dags:ro
      - {{ .AirflowHome }}/plugins:/usr/local/airflow/plugins:{{ .MountLabel }}
      - {{ .AirflowHome }}/include:/usr/local/airflow/include:{{ .MountLabel }}
      - airflow_logs:/usr/local/airflow/logs
    {{ .AirflowEnvFile }}

  webserver:
    image: {{ .AirflowImage }}
    command: >
      bash -c 'airflow create_user "$$@" || airflow users create "$$@"' -- -r Admin -u admin -e admin@example.com -f admin -l user -p admin && airflow webserver
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
      AIRFLOW__CORE__SQL_ALCHEMY_CONN: postgresql://{{ .PostgresUser }}:{{ .PostgresPassword }}@{{ .PostgresHost }}:5432
      AIRFLOW__CORE__LOAD_EXAMPLES: "False"
      AIRFLOW__CORE__FERNET_KEY: "d6Vefz3G9U_ynXB3cr7y_Ak35tAHkEGAVxuz_B-jzWw="
      AIRFLOW__WEBSERVER__RBAC: "True"
    ports:
      - {{ .AirflowWebserverPort }}:8080
    volumes:
      - {{ .AirflowHome }}/dags:/usr/local/airflow/dags:{{ .MountLabel }}
      - {{ .AirflowHome }}/plugins:/usr/local/airflow/plugins:{{ .MountLabel }}
      - {{ .AirflowHome }}/include:/usr/local/airflow/include:{{ .MountLabel }}
      - airflow_logs:/usr/local/airflow/logs
    {{ .AirflowEnvFile }}
`)
