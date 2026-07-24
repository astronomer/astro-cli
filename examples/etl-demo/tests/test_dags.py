"""Parse test: load every DAG in dags/ and fail if any of them has an import error.

This is the Airflow 3 form of the classic dagbag integrity test. `astro local check` runs
the same idea from the CLI; this keeps it in the test suite so `uv run pytest` (or
`astro local run pytest`) catches a broken DAG too.
"""

from __future__ import annotations

from pathlib import Path

from airflow.models.dagbag import DagBag

DAGS_DIR = Path(__file__).resolve().parent.parent / "dags"


def test_dags_import_without_errors() -> None:
    dagbag = DagBag(dag_folder=str(DAGS_DIR), include_examples=False)
    assert not dagbag.import_errors, f"DAG import errors: {dagbag.import_errors}"


def test_expected_dags_are_present() -> None:
    dagbag = DagBag(dag_folder=str(DAGS_DIR), include_examples=False)
    assert set(dagbag.dag_ids) == {"etl_taskflow", "branch_example", "dynamic_mapping"}
