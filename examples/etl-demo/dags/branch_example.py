"""A @task.branch DAG: one task picks which downstream task runs based on a value."""

from __future__ import annotations

from datetime import datetime

from airflow.sdk import dag, task


@dag(
    schedule="@daily",
    start_date=datetime(2024, 1, 1),
    catchup=False,
    tags=["example", "branch"],
)
def branch_example():
    @task
    def count_rows() -> int:
        """Stand in for a real check: how many rows arrived."""
        return 42

    @task.branch
    def choose(row_count: int) -> str:
        """Return the task_id to run next."""
        if row_count > 0:
            return "process"
        return "skip"

    @task
    def process() -> None:
        print("rows present, processing")

    @task
    def skip() -> None:
        print("no rows, nothing to do")

    rows = count_rows()
    branch = choose(rows)
    branch >> [process(), skip()]


branch_example()
