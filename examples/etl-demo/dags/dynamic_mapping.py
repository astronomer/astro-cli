"""A dynamic task-mapping DAG.

One task fans out over a list with .expand(), and a follow-up collects the results.
"""

from __future__ import annotations

from datetime import datetime

from airflow.sdk import dag, task


@dag(
    schedule="@daily",
    start_date=datetime(2024, 1, 1),
    catchup=False,
    tags=["example", "dynamic"],
)
def dynamic_mapping():
    @task
    def list_files() -> list[str]:
        """Stand in for a real listing: the files to process this run."""
        return ["a.csv", "b.csv", "c.csv"]

    @task
    def row_count(filename: str) -> int:
        """Process one file. Airflow runs one copy of this task per file."""
        return len(filename)

    @task
    def total(counts: list[int]) -> None:
        print(f"total rows: {sum(counts)}")

    counts = row_count.expand(filename=list_files())
    total(counts)


dynamic_mapping()
