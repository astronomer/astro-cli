"""The classic parse test: every DAG in dags/ imports cleanly.

`astro local check` does this and more from the CLI. This is the same guard as
a pytest, so CI that already runs pytest keeps it.
"""

from airflow.models import DagBag


def test_dags_import_without_errors():
    dagbag = DagBag(include_examples=False)
    assert not dagbag.import_errors, dagbag.import_errors
    assert dagbag.dags, "no DAGs were found"
