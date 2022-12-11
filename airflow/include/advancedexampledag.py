from datetime import datetime, timedelta
from typing import Dict

# Airflow operators are templates for tasks and encompass the logic that your DAG will actually execute.
# To use an operator in your DAG, you first have to import it.
# To learn more about operators, see: https://registry.astronomer.io/.

from airflow.decorators import dag, task # DAG and task decorators for interfacing with the TaskFlow API
from airflow.models.baseoperator import chain # A function that sets sequential dependencies between tasks including lists of tasks.
from airflow.operators.bash import BashOperator
from airflow.operators.dummy import DummyOperator
from airflow.operators.email import EmailOperator
from airflow.operators.python import BranchPythonOperator
from airflow.operators.weekday import BranchDayOfWeekOperator
from airflow.utils.edgemodifier import Label # Used to label node edges in the Airflow UI
from airflow.utils.task_group import TaskGroup # Used to group tasks together in the Graph view of the Airflow UI
from airflow.utils.trigger_rule import TriggerRule # Used to change how an Operator is triggered
from airflow.utils.weekday import WeekDay # Used to determine what day of the week it is


"""
This DAG is intended to demonstrate a number of core Apache Airflow concepts that are central to the pipeline
authoring experience, including the TaskFlow API, Edge Labels, Jinja templating, branching,
dynamic task generation, Task Groups, and Trigger Rules.

First, this DAG checks if the current day is a weekday or weekend. Next, the DAG  checks which day of the week
it is. Lastly, the DAG prints out a bash statement based on which day it is. On Tuesday, for example, the DAG
prints "It's Tuesday and I'm busy with studying".

This DAG uses the following operators:

BashOperator -
    Executes a bash script or bash command.

    See more info about this operator here:
        https://registry.astronomer.io/providers/apache-airflow/modules/bashoperator

DummyOperator -
    Does nothing but can be used to group tasks in a DAG

    See more info about this operator here:
        https://registry.astronomer.io/providers/apache-airflow/modules/dummyoperator

EmailOperator -
    Used to send emails

    See more info about this operator here:
        https://registry.astronomer.io/providers/apache-airflow/modules/emailoperator

BranchPythonOperator -
    Allows a workflow to “branch” after a task based on the result of a Python function

    See more info about this operator here:
        https://registry.astronomer.io/providers/apache-airflow/modules/branchpythonoperator

BranchDayOfWeekOperator -
    Branches into one of two lists of tasks depending on the current day

    See more info about this operator here:
        https://registry.astronomer.io/providers/apache-airflow/modules/branchdayofweekoperator
"""

# Reference data that defines "weekday" as well as the activity assigned to each day of the week.
DAY_ACTIVITY_MAPPING = {
    "monday": {"is_weekday": True, "activity": "guitar lessons"},
    "tuesday": {"is_weekday": True, "activity": "studying"},
    "wednesday": {"is_weekday": True, "activity": "soccer practice"},
    "thursday": {"is_weekday": True, "activity": "contributing to Airflow"},
    "friday": {"is_weekday": True, "activity": "family dinner"},
    "saturday": {"is_weekday": False, "activity": "going to the beach"},
    "sunday": {"is_weekday": False, "activity": "sleeping in"},
}


@task(multiple_outputs=True) # multiple_outputs=True unrolls dictionaries into separate XCom values
def _going_to_the_beach() -> Dict:
    return {
        "subject": "Beach day!",
        "body": "It's Saturday and I'm heading to the beach.<br><br>Come join me!<br>",
    }


# This functions gets the activity from the "DAY_ACTIVITY_MAPPING" dictionary
def _get_activity(day_name) -> str:
    activity_id = DAY_ACTIVITY_MAPPING[day_name]["activity"].replace(" ", "_")

    if DAY_ACTIVITY_MAPPING[day_name]["is_weekday"]:
        return f"weekday_activities.{activity_id}"

    return f"weekend_activities.{activity_id}"


# When using the DAG decorator, the "dag" argument doesn't need to be specified for each task.
# The "dag_id" value defaults to the name of the function it is decorating if not explicitly set.
# In this example, the "dag_id" value would be "example_dag_advanced".
@dag(
    # This DAG is set to run for the first time on June 11, 2021. Best practice is to use a static start_date.
    # Subsequent DAG runs are instantiated based on scheduler_interval below.
    start_date=datetime(2021, 6, 11),
    # This defines how many instantiations of this DAG (DAG Runs) can execute concurrently. In this case,
    # we're only allowing 1 DAG run at any given time, as opposed to allowing multiple overlapping DAG runs.
    max_active_runs=1,
    # This defines how often your DAG will run, or the schedule by which DAG runs are created. It can be
    # defined as a cron expression or custom timetable. This DAG will run daily.
    schedule_interval="@daily",
    # Default settings applied to all tasks within the DAG; can be overwritten at the task level.
    default_args={
        "owner": "community", # This defines the value of the "owner" column in the DAG view of the Airflow UI
        "retries": 2, # If a task fails, it will retry 2 times.
        "retry_delay": timedelta(minutes=3), # A task that fails will wait 3 minutes to retry.
    },
    default_view="graph", # This defines the default view for this DAG in the Airflow UI
    # When catchup=False, your DAG will only run for the latest schedule interval. In this case, this means
    # that tasks will not be run between June 11, 2021 and 1 day ago. When turned on, this DAG's first run
    # will be for today, per the @daily schedule interval
    catchup=False,
    tags=["example"], # If set, this tag is shown in the DAG view of the Airflow UI
)
def example_dag_advanced():
    # DummyOperator placeholder for first task
    begin = DummyOperator(task_id="begin")
    # Last task will only trigger if no previous task failed
    end = DummyOperator(task_id="end", trigger_rule=TriggerRule.NONE_FAILED)

    # This task checks which day of the week it is
    check_day_of_week = BranchDayOfWeekOperator(
        task_id="check_day_of_week",
        week_day={WeekDay.SATURDAY, WeekDay.SUNDAY}, # This checks day of week
        follow_task_ids_if_true="weekend", # Next task if criteria is met
        follow_task_ids_if_false="weekday", # Next task if criteria is not met
        use_task_execution_day=True, # If True, uses task’s execution day to compare with is_today
    )

    weekend = DummyOperator(task_id="weekend") # "weekend" placeholder task
    weekday = DummyOperator(task_id="weekday") # "weekday" placeholder task

    # Templated value for determining the name of the day of week based on the start date of the DAG Run
    day_name = "{{ dag_run.start_date.strftime('%A').lower() }}"

    # Begin weekday tasks.
    # Tasks within this TaskGroup (weekday tasks) will be grouped together in the Airflow UI
    with TaskGroup("weekday_activities") as weekday_activities:
        which_weekday_activity_day = BranchPythonOperator(
            task_id="which_weekday_activity_day",
            python_callable=_get_activity, # Python function called when task executes
            op_args=[day_name],
        )

        for day, day_info in DAY_ACTIVITY_MAPPING.items():
            if day_info["is_weekday"]:
                day_of_week = Label(label=day)
                activity = day_info["activity"]

                # This task prints the weekday activity to bash
                do_activity = BashOperator(
                    task_id=activity.replace(" ", "_"),
                    bash_command=f"echo It's {day.capitalize()} and I'm busy with {activity}.", # This is the bash command to run
                )

                # Declaring task dependencies within the "TaskGroup" via the classic bitshift operator.
                which_weekday_activity_day >> day_of_week >> do_activity

    # Begin weekend tasks
    # Tasks within this TaskGroup will be grouped together in the UI
    with TaskGroup("weekend_activities") as weekend_activities:
        which_weekend_activity_day = BranchPythonOperator(
            task_id="which_weekend_activity_day",
            python_callable=_get_activity, # Python function called when task executes
            op_args=[day_name],
        )

        # Labels that will appear in the Graph view of the Airflow UI
        saturday = Label(label="saturday")
        sunday = Label(label="sunday")

        # This task prints the Sunday activity to bash
        sleeping_in = BashOperator(task_id="sleeping_in", bash_command="sleep $[ ( $RANDOM % 30 )  + 1 ]s")

        going_to_the_beach = _going_to_the_beach() # Calling the taskflow function

        # Because the "_going_to_the_beach()" function has "multiple_outputs" enabled, each dict key is
        # accessible as their own "XCom" key.
        inviting_friends = EmailOperator(
            task_id="inviting_friends",
            to="friends@community.com", # Email to send email to
            subject=going_to_the_beach["subject"], # Email subject
            html_content=going_to_the_beach["body"], # Eamil body content
        )

        # Using "chain()" here for list-to-list dependencies which are not supported by the bitshift
        # operator and to simplify the notation for the desired dependency structure.
        chain(which_weekend_activity_day, [saturday, sunday], [going_to_the_beach, sleeping_in])

    # High-level dependencies between tasks
    chain(begin, check_day_of_week, [weekday, weekend], [weekday_activities, weekend_activities], end)

    # Task dependency created by XComArgs:
    # going_to_the_beach >> inviting_friends

dag = example_dag_advanced()