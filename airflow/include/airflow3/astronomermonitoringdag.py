"""A Monitoring DAG used and maintained by Astronomer. All tasks in this DAG are executed by workers in the default worker queue. When tasks in the Monitoring DAG fail, it might indicate a problem with your Deployment. Astronomer will alert you via email."""

import os
from datetime import timedelta

import pendulum
from airflow import DAG
from airflow.providers.standard.operators.bash import BashOperator

with DAG(
    dag_id="astronomer_monitoring_dag",
    schedule=os.environ.get("AIRFLOW_MONITORING_DAG_SCHEDULE_INTERVAL", "*/5 * * * *"),
    catchup=False,
    is_paused_upon_creation=os.environ.get("PAUSE_ASTRONOMER_MONITORING_DAG", "False").lower() == "true",
    dagrun_timeout=timedelta(minutes=30),
    description=__doc__,
    doc_md=__doc__,
    tags=["monitoring"],
):
    hello = BashOperator(
        task_id="hello",
        bash_command="echo Hello from Astronomer!",
        depends_on_past=False,
        priority_weight=2**31 - 1,
        do_xcom_push=False,
    )
