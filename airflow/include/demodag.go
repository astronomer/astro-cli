package include

import "strings"

var Demodag = strings.TrimSpace(`
from airflow import DAG
from airflow.utils.dates import days_ago

import random

with DAG(dag_id='mapping_example', schedule_interval='@hourly', start_date=days_ago(2), catchup=False) as dag:

    @dag.task
    def make_list(data_interval_start=None, **kwargs):
        return [i + 1 for i in range(data_interval_start.hour)]

    @dag.task
    def consumer(value):
        print(repr(value))

    consumer.expand(value=make_list())
`)
