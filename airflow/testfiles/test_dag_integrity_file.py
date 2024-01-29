"""This is a test file, content of the file isn't important as much as the existence of the file itself"""

"""Test the validity of all DAGs. **USED BY DEV PARSE COMMAND DO NOT EDIT**"""
from contextlib import contextmanager
import logging
import os

import pytest

from airflow.models import DagBag, Variable, Connection
from airflow.hooks.base import BaseHook


@contextmanager
def suppress_logging(namespace):
    """
    Suppress logging within a specific namespace to keep tests "clean" during build
    """
    logger = logging.getLogger(namespace)
    old_value = logger.disabled
    logger.disabled = True
    try:
        yield
    finally:
        logger.disabled = old_value


def get_import_errors():
    """
    Generate a tuple for import errors in the dag bag
    """
    with suppress_logging("airflow"):
        dag_bag = DagBag(include_examples=False)

        def strip_path_prefix(path):
            return os.path.relpath(path, os.environ.get("AIRFLOW_HOME"))

        # prepend "(None,None)" to ensure that a test object is always created even if it's a no op.
        return [(None, None)] + [
            (strip_path_prefix(k), v.strip()) for k, v in dag_bag.import_errors.items()
        ]


@pytest.mark.parametrize(
    "rel_path,rv", get_import_errors(), ids=[x[0] for x in get_import_errors()]
)
def test_file_imports(rel_path, rv):
    """Test for import errors on a file"""
    if rel_path and rv:  # Make sure our no op test doesn't raise an error
        raise Exception(f"{rel_path} failed to import with message \n {rv}")
