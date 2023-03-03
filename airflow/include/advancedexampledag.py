from pendulum import datetime, duration

# Airflow Operators are templates for tasks and encompass the logic that your DAG will actually execute.
# To use an operator in your DAG, you first have to import it.
# To learn more about operators, see: https://registry.astronomer.io/.

# DAG and task decorators for interfacing with the TaskFlow API
from airflow.decorators import dag, task, task_group

# A function that sets sequential dependencies between tasks including lists of tasks
from airflow.models.baseoperator import chain

from airflow.operators.bash import BashOperator
from airflow.operators.empty import EmptyOperator
from airflow.operators.weekday import BranchDayOfWeekOperator

# Used to label node edges in the Airflow UI
from airflow.utils.edgemodifier import Label

# Used to determine the day of the week
from airflow.utils.weekday import WeekDay


"""
This DAG is intended to demonstrate a number of core Apache Airflow concepts that are central to the pipeline
authoring experience, including the TaskFlow API, Edge Labels, Jinja templating, branching,
generating tasks within a loop, task groups, and trigger rules.

First, this DAG checks if the current day is a weekday or weekend. Next, the DAG checks which day of the week
it is. Lastly, the DAG prints out a bash statement based on which day it is. On Tuesday, for example, the DAG
prints "It's Tuesday and I'm busy with studying".

This DAG uses the following operators:

BashOperator -
    Executes a Bash script, command, or set of commands.

    See more info about this operator here:
        https://registry.astronomer.io/providers/apache-airflow/modules/bashoperator

EmptyOperator -
    Does nothing but can be used to structure your DAG.

    See more info about this operator here:
        https://registry.astronomer.io/providers/apache-airflow/modules/emptyoperator

BranchDayOfWeekOperator -
    Branches into one of two lists of tasks depending on the current day.

    See more info about this operator here:
        https://registry.astronomer.io/providers/apache-airflow/modules/branchdayofweekoperator
"""

# Reference data that defines "weekday" as well as the activity assigned to each day of the week
DAY_ACTIVITY_MAPPING = {
    "monday": {"is_weekday": True, "activity": "guitar lessons"},
    "tuesday": {"is_weekday": True, "activity": "studying"},
    "wednesday": {"is_weekday": True, "activity": "soccer practice"},
    "thursday": {"is_weekday": True, "activity": "contributing to Airflow"},
    "friday": {"is_weekday": True, "activity": "family dinner"},
    "saturday": {"is_weekday": False, "activity": "going to the beach"},
    "sunday": {"is_weekday": False, "activity": "sleeping in"},
}

# The TaskFlow API is also used in a number of tasks within this DAG. Check out of the TaskFlow API tutorial
# to learn more.
#   https://airflow.apache.org/docs/apache-airflow/stable/tutorial/taskflow.html


# This is the TaskFlow equivalent of the PythonOperator:
#   https://registry.astronomer.io/providers/apache-airflow/modules/pythonoperator
@task(
    # By default the function name is used as the `task_id`, but it can be overriden if desired.
    task_id="going_to_the_beach",
    multiple_outputs=True,  # multiple_outputs=True unrolls dictionaries into separate XCom values
)
def _going_to_the_beach() -> dict[str, str]:
    return {
        "subject": "Beach day!",
        "body": "It's Saturday and I'm heading to the beach.<br>Come join me!",
    }


# This is the TaskFlow API equivalent to the BranchPythonOperator:
#   https://registry.astronomer.io/providers/apache-airflow/modules/branchpythonoperator
# The task retrieves the activity from the "DAY_ACTIVITY_MAPPING" dictionary.
@task.branch
def get_activity(day_name: str) -> str:
    activity_id = DAY_ACTIVITY_MAPPING[day_name]["activity"].replace(" ", "_")

    if DAY_ACTIVITY_MAPPING[day_name]["is_weekday"]:
        return f"weekday_activities.{activity_id}"

    return f"weekend_activities.{activity_id}"


# This the TaskFlow API equivalent to the PythonVirtualEnvOperator:
#   https://registry.astronomer.io/providers/apache-airflow/modules/pythonvirtualenvoperator
@task.virtualenv(requirements=["beautifulsoup4==4.11.2"])
def inviting_friends(subject: str, body: str) -> None:
    from bs4 import BeautifulSoup

    print("Inviting friends...")
    html_doc = f"<title>{subject}</title><p>{body}</p>"
    soup = BeautifulSoup(html_doc, "html.parser")
    print(soup.prettify())


# When using the DAG decorator, the "dag" argument doesn't need to be specified for each task.
# The "dag_id" value defaults to the name of the function it is decorating if not explicitly set.
# In this example, the "dag_id" value would be "example_dag_advanced".
@dag(
    # This DAG is set to run for the first time on January 1, 2023.
    # Best practice is to use a static start_date.
    # Subsequent DAG runs are instantiated based on the "schedule" parameter below.
    start_date=datetime(2023, 1, 1),
    # This defines how many instantiations of this DAG (DAG Runs) can execute concurrently. In this case,
    # we're only allowing 1 DAG run at any given time, as opposed to allowing multiple overlapping DAG runs.
    max_active_runs=1,
    # This defines how often your DAG will run, or the schedule by which DAG runs are created. It can be
    # defined as a cron expression, custom timetable, existing presets or using the Dataset feature.
    # This DAG uses a preset to run daily.
    schedule="@daily",
    # Default settings applied to all tasks within the DAG; can be overwritten at the task level.
    default_args={
        "owner": "community",  # Defines the value of the "owner" column in the DAG view of the Airflow UI
        "retries": 2,  # If a task fails, it will retry 2 times.
        "retry_delay": duration(
            minutes=3
        ),  # A task that fails will wait 3 minutes to retry.
    },
    default_view="graph",  # This defines the default view for this DAG in the Airflow UI
    # When catchup=False, your DAG will only run for the latest schedule interval. In this case, this means
    # that tasks will not be run between January 1st, 2023 and 1 day ago. When turned on, this DAG's first run
    # will be for today, per the @daily schedule
    catchup=False,
    tags=["example"],  # If set, this tag is shown in the DAG view of the Airflow UI
)
def example_dag_advanced():
    # EmptyOperator placeholder for first task
    begin = EmptyOperator(task_id="begin")
    # Last task will only trigger if all upstream tasks have succeeded or been skipped
    end = EmptyOperator(task_id="end", trigger_rule="none_failed")

    # This task checks which day of the week it is
    check_day_of_week = BranchDayOfWeekOperator(
        task_id="check_day_of_week",
        week_day={WeekDay.SATURDAY, WeekDay.SUNDAY},  # This checks day of week
        follow_task_ids_if_true="weekend",  # Next task if criteria is met
        follow_task_ids_if_false="weekday",  # Next task if criteria is not met
        use_task_execution_day=True,  # If True, uses task’s execution day to compare with is_today
    )

    weekend = EmptyOperator(task_id="weekend")  # "weekend" placeholder task
    weekday = EmptyOperator(task_id="weekday")  # "weekday" placeholder task

    # Templated value for determining the name of the day of week based on the start date of the DAG Run
    day_name = "{{ dag_run.start_date.strftime('%A').lower() }}"

    # Begin weekday tasks.
    # Tasks within this TaskGroup (weekday tasks) will be grouped together in the Airflow UI
    @task_group
    def weekday_activities():
        # TaskFlow functions can also be reused which is beneficial if you want to use the same callable for
        # multiple tasks and want to use different task attributes.
        # See this tutorial for more information:
        #   https://airflow.apache.org/docs/apache-airflow/stable/tutorial/taskflow.html#reusing-a-decorated-task
        which_weekday_activity_day = get_activity.override(
            task_id="which_weekday_activity_day"
        )(day_name)

        for day, day_info in DAY_ACTIVITY_MAPPING.items():
            if day_info["is_weekday"]:
                day_of_week = Label(label=day)
                activity = day_info["activity"]

                # This task prints the weekday activity to bash
                do_activity = BashOperator(
                    task_id=activity.replace(" ", "_"),
                    # This is the Bash command to run
                    bash_command=f"echo It's {day.capitalize()} and I'm busy with {activity}.",
                )

                # Declaring task dependencies within the "TaskGroup" via the classic bitshift operator.
                which_weekday_activity_day >> day_of_week >> do_activity

    # Begin weekend tasks
    # Tasks within this TaskGroup will be grouped together in the UI
    @task_group
    def weekend_activities():
        which_weekend_activity_day = get_activity.override(
            task_id="which_weekend_activity_day"
        )(day_name)

        # Labels that will appear in the Graph view of the Airflow UI
        saturday = Label(label="saturday")
        sunday = Label(label="sunday")

        # This task runs the Sunday activity of sleeping for a random interval between 1 and 30 seconds
        sleeping_in = BashOperator(
            task_id="sleeping_in", bash_command="sleep $[ (1 + $RANDOM % 30) ]s"
        )

        going_to_the_beach = _going_to_the_beach()  # Calling the TaskFlow task

        # Because the "_going_to_the_beach()" function has "multiple_outputs" enabled, each dict key is
        # accessible as their own "XCom" key.
        _inviting_friends = inviting_friends(
            subject=going_to_the_beach["subject"], body=going_to_the_beach["body"]
        )

        # Using "chain()" here for list-to-list dependencies which are not supported by the bitshift
        # operator and to simplify the notation for the desired dependency structure.
        chain(
            which_weekend_activity_day,
            [saturday, sunday],
            [going_to_the_beach, sleeping_in],
        )

    # Call the @task_group TaskFlow functions to instantiate them in the DAG
    _weekday_activities = weekday_activities()
    _weekend_activities = weekend_activities()

    # High-level dependencies between tasks
    chain(
        begin,
        check_day_of_week,
        [weekday, weekend],
        [_weekday_activities, _weekend_activities],
        end,
    )

    # Task dependency created by XComArgs:
    # going_to_the_beach >> inviting_friends


example_dag_advanced()
