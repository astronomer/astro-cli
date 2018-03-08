package airflow

import "strings"

// dockerfile is the Dockerfile template
var dockerfile = strings.TrimSpace(`
FROM astronomerinc/ap-airflow:latest-onbuild
`)

// dockerignore is the .dockerignore template
var dockerignore = strings.TrimSpace(`
.astro
.git
`)

// airflow example dag
var exampledag = strings.TrimSpace(`
from airflow import DAG
from airflow.operators.bash_operator import BashOperator
from datetime import datetime, timedelta

default_args = {
    'owner': 'airflow',
    'depends_on_past': False,
    'start_date': datetime(2018, 1, 1),
    'email_on_failure': False,
    'email_on_retry': False,
    'retries': 1,
    'retry_delay': timedelta(minutes=5),
}

dag = DAG('example_dag',
          schedule_interval=timedelta(minutes=5),
          default_args=default_args)

t1 = BashOperator(
    task_id='print_date1',
    bash_command='sleep 2m',
    dag=dag)

t2 = BashOperator(
    task_id='print_date2',
    bash_command='sleep 2m',
    dag=dag)

t3 = BashOperator(
    task_id='print_date3',
    bash_command='sleep 2m',
    dag=dag)

t4 = BashOperator(
    task_id='print_date4',
    bash_command='sleep 2m',
    dag=dag)

t5 = BashOperator(
    task_id='print_date5',
    bash_command='sleep 2m',
    dag=dag)

t6 = BashOperator(
    task_id='print_date6',
    bash_command='sleep 2m',
    dag=dag)

t7 = BashOperator(
    task_id='print_date7',
    bash_command='sleep 2m',
    dag=dag)

t8 = BashOperator(
    task_id='print_date8',
    bash_command='sleep 2m',
    dag=dag)

t2.set_upstream(t1)
t3.set_upstream(t2)
t4.set_upstream(t3)
t5.set_upstream(t3)
t6.set_upstream(t3)
t7.set_upstream(t3)
t8.set_upstream(t3)

t9 = BashOperator(
    task_id='print_date9',
    bash_command='sleep 2m',
    dag=dag)

t10 = BashOperator(
    task_id='print_date10',
    bash_command='sleep 2m',
    dag=dag)

t11 = BashOperator(
    task_id='print_date11',
    bash_command='sleep 2m',
    dag=dag)

t12 = BashOperator(
    task_id='print_date12',
    bash_command='sleep 2m',
    dag=dag)

t9.set_upstream(t8)
t10.set_upstream(t8)
t11.set_upstream(t8)
t12.set_upstream(t8)
`)

// composeyml is the docker-compose template
var composeyml = strings.TrimSpace(`
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
