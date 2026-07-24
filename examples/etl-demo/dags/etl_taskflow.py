"""A small TaskFlow ETL.

Extract, transform, load, with values passed between tasks as return values.
"""

from __future__ import annotations

from datetime import datetime

from airflow.sdk import dag, task


@dag(
    schedule="@daily",
    start_date=datetime(2024, 1, 1),
    catchup=False,
    tags=["example", "etl"],
)
def etl_taskflow():
    @task
    def extract() -> list[tuple[str, float]]:
        """Stand in for a real source: return a few (region, amount) rows."""
        return [("us", 10.0), ("us", 5.0), ("eu", 7.0)]

    @task
    def transform(rows: list[tuple[str, float]]) -> dict[str, float]:
        """Sum the amounts per region."""
        totals: dict[str, float] = {}
        for region, amount in rows:
            totals[region] = totals.get(region, 0.0) + amount
        return totals

    @task
    def load(totals: dict[str, float]) -> None:
        """Stand in for a real sink: print the result."""
        for region, total in sorted(totals.items()):
            print(f"{region}: {total}")

    load(transform(extract()))


etl_taskflow()
