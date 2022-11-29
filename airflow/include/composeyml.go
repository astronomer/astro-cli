package include

import "strings"

// Composeyml is the docker-compose template
var Composeyml = strings.TrimSpace(`
version: '3.1'

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
    image: postgres:12.6
    restart: unless-stopped
    networks:
      - airflow
    labels:
      io.astronomer.docker: "true"
      io.astronomer.docker.cli: "true"
    ports:
      - 127.0.0.1:{{ .PostgresPort }}:5432
    volumes:
      - postgres_data:/var/lib/postgresql/data
    environment:
      POSTGRES_USER: {{ .PostgresUser }}
      POSTGRES_PASSWORD: {{ .PostgresPassword }}

  scheduler:
    image: {{ .AirflowImage }}
    command: >
      bash -c "(airflow db upgrade || airflow upgradedb) && airflow scheduler"
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
      ASTRONOMER_ENVIRONMENT: local
    volumes:
      - {{ .AirflowHome }}/dags:/usr/local/airflow/dags:ro
      - {{ .AirflowHome }}/plugins:/usr/local/airflow/plugins:{{ .MountLabel }}
      - {{ .AirflowHome }}/include:/usr/local/airflow/include:{{ .MountLabel }}
      - {{ .AirflowHome }}/tests:/usr/local/airflow/tests:{{ .MountLabel }}
{{if .SettingsFileExist}}
      - {{ .AirflowHome }}/{{ .SettingsFile }}:/usr/local/airflow/{{ .SettingsFile }}:{{ .MountLabel }}
{{end}}
      - airflow_logs:/usr/local/airflow/logs
    {{ .AirflowEnvFile }}

  webserver:
    image: {{ .AirflowImage }}
    command: >
      bash -c 'if [[ -z "$$AIRFLOW__API__AUTH_BACKEND" ]] && [[ $$(pip show -f apache-airflow | grep basic_auth.py) ]];
        then export AIRFLOW__API__AUTH_BACKEND=airflow.api.auth.backend.basic_auth ;
        else export AIRFLOW__API__AUTH_BACKEND=airflow.api.auth.backend.default ; fi &&
        { airflow users create "$$@" || airflow create_user "$$@" ; } &&
        { airflow sync-perm || airflow sync_perm ;} &&
        airflow webserver' -- -r Admin -u admin -e admin@example.com -f admin -l user -p admin
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
      ASTRONOMER_ENVIRONMENT: local
    ports:
      - 127.0.0.1:{{ .AirflowWebserverPort }}:8080
    volumes:
      - {{ .AirflowHome }}/dags:/usr/local/airflow/dags:{{ .MountLabel }}
      - {{ .AirflowHome }}/plugins:/usr/local/airflow/plugins:{{ .MountLabel }}
      - {{ .AirflowHome }}/include:/usr/local/airflow/include:{{ .MountLabel }}
      - {{ .AirflowHome }}/tests:/usr/local/airflow/tests:{{ .MountLabel }}
      - airflow_logs:/usr/local/airflow/logs
    healthcheck:
      test: curl --fail http://webserver:8080/health || exit 1
      interval: 2s
      retries: 15
      start_period: 5s
      timeout: 60s
    {{ .AirflowEnvFile }}
{{if .TriggererEnabled}}
  triggerer:
    image: {{ .AirflowImage }}
    command: >
      bash -c "(airflow db upgrade || airflow upgradedb) && airflow triggerer"
    restart: unless-stopped
    networks:
      - airflow
    user: {{ .AirflowUser }}
    labels:
      io.astronomer.docker: "true"
      io.astronomer.docker.cli: "true"
      io.astronomer.docker.component: "airflow-triggerer"
    depends_on:
      - postgres
    environment:
      AIRFLOW__CORE__EXECUTOR: LocalExecutor
      AIRFLOW__CORE__SQL_ALCHEMY_CONN: postgresql://{{ .PostgresUser }}:{{ .PostgresPassword }}@{{ .PostgresHost }}:5432
      AIRFLOW__CORE__LOAD_EXAMPLES: "False"
      AIRFLOW__CORE__FERNET_KEY: "d6Vefz3G9U_ynXB3cr7y_Ak35tAHkEGAVxuz_B-jzWw="
      AIRFLOW__WEBSERVER__RBAC: "True"
    volumes:
      - {{ .AirflowHome }}/dags:/usr/local/airflow/dags:{{ .MountLabel }}
      - {{ .AirflowHome }}/plugins:/usr/local/airflow/plugins:{{ .MountLabel }}
      - {{ .AirflowHome }}/include:/usr/local/airflow/include:{{ .MountLabel }}
      - airflow_logs:/usr/local/airflow/logs
    {{ .AirflowEnvFile }}
{{end}}
`)
