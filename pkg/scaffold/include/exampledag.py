"""
## Example DAG

A small pipeline written with Airflow's TaskFlow API. One task returns a list,
the next runs once per item in it, and the dependency between them is inferred
from the function call rather than wired up by hand.

It imports nothing but `airflow.sdk` and the standard library, so it runs on a
new project as scaffolded and works offline. That is the shape of an Astro
project: `[project.dependencies]` in pyproject.toml declares what this project
installs, so add a library there when a task needs one, then `astro local start`
to pick it up.

For a longer walkthrough, see the getting started tutorial. It covers calling an
API, assets, and scheduling one DAG from another.
https://www.astronomer.io/docs/learn/get-started-with-airflow
"""

from datetime import datetime, timezone

from airflow.sdk import dag, task


# The basic parameters of the DAG: when it starts, and how often it runs.
@dag(
    # Give the start date a timezone rather than leaving it naive. A naive one
    # is read using whatever core.default_timezone happens to say, so the DAG
    # would start somewhere different on a differently configured Airflow.
    start_date=datetime(2026, 1, 1, tzinfo=timezone.utc),
    schedule="@daily",
    catchup=False,
    doc_md=__doc__,
    default_args={"owner": "Astro", "retries": 3},
    tags=["example"],
)
def example_dag():
    @task
    def get_names() -> list[str]:
        """Return the values the next task works on.

        A task's return value is passed to whatever uses it, through XCom.
        Returning a list is what lets the next task fan out over it.
        """
        return ["Ada", "Grace", "Katherine"]

    @task
    def greet(name: str) -> str:
        """Greet one name.

        This runs once per name, as a separate task instance. The list is not
        known until get_names has run, so Airflow creates the copies while the
        DAG is running rather than when it is parsed.
        """
        greeting = f"Hello, {name}!"
        print(greeting)
        return greeting

    # expand() creates one greet() per item get_names() returned, and passing
    # the call directly is what declares get_names -> greet.
    greet.expand(name=get_names())


# Instantiate the DAG.
example_dag()
