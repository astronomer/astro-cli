# Embedded by internal/checks and run with the project's own .venv Python
# (`python - <project_root> <dags_dir>`, program on stdin). It builds one
# Airflow DagBag and prints a single JSON object to stdout: per-DAG import
# errors, the loaded DAG inventory, and per-file parse times. No scheduler,
# no metadata database, no writes to the user's project — AIRFLOW_HOME points
# at a throwaway directory the Go caller owns.
#
# This modernizes the v1 "DO NOT EDIT" integrity-test file, which pytest ran
# from a copy written into the user's tree. Here the checks run from this
# embedded copy and the decisions (which findings, which exit code) are the
# Go caller's; this script only reports structured facts.
#
# The contract with the Go side is the JSON shape below. It always exits 0
# when it can emit valid JSON: a missing/broken Airflow is reported in the
# "fatal" field rather than through the exit code, so the caller never has to
# read meaning into an exit status (the an earlier fix lesson).
import ast
import json
import logging
import os
import sys


def emit(result):
    json.dump(result, sys.stdout)
    sys.stdout.write("\n")


def install_parse_monkeypatches():
    # DAGs commonly read connections, Variables, and env vars at import time.
    # A real run has them; a parse check must not fail just because they are
    # absent, so return benign stand-ins instead. Lifted and trimmed from the
    # v1 integrity template. Each patch is guarded: an Airflow layout that
    # lacks one of these symbols simply skips that patch.
    try:
        from airflow.hooks.base import BaseHook
        from airflow.models import Connection

        def _get_connection(key, *args, **kwargs):
            return Connection(key)

        BaseHook.get_connection = staticmethod(_get_connection)
    except Exception:
        pass

    _no_default = object()

    class _MagicDict(dict):
        def __getitem__(self, key):
            return "MOCKED_KEY_VALUE"

    def _variable_get(key, default_var=_no_default, deserialize_json=False):
        if default_var is not _no_default:
            return default_var
        if deserialize_json:
            return _MagicDict()
        return "MOCKED_VARIABLE_VALUE"

    for module_name, attr in (("airflow.models", "Variable"), ("airflow.sdk", "Variable")):
        try:
            module = __import__(module_name, fromlist=[attr])
            getattr(module, attr).get = staticmethod(_variable_get)
        except Exception:
            pass

    real_getenv = os.getenv

    def _getenv(key, *args, **kwargs):
        value = real_getenv(key, *args, **kwargs)
        if value is not None:
            return value
        if args:
            return args[0]
        if "default" in kwargs:
            return kwargs["default"]
        return "MOCKED_{}_VALUE".format(key.upper())

    os.getenv = _getenv


def file_dag_ids(stat):
    # FileLoadStat.dags is a str repr of a list in older Airflow and an
    # iterable in newer ones; accept both, and never let one odd stat abort
    # the whole report.
    dags = getattr(stat, "dags", None)
    if isinstance(dags, str):
        try:
            dags = ast.literal_eval(dags)
        except Exception:
            return []
    try:
        return [str(d) for d in dags]
    except TypeError:
        return []


def parse_seconds(stat):
    duration = getattr(stat, "duration", 0)
    total = getattr(duration, "total_seconds", None)
    if callable(total):
        return total()
    try:
        return float(duration)
    except (TypeError, ValueError):
        return 0.0


def main():
    project_root = sys.argv[1] if len(sys.argv) > 1 else os.getcwd()
    dags_dir = sys.argv[2] if len(sys.argv) > 2 else os.path.join(project_root, "dags")

    # Resolve `include`/`plugins` style imports the way a running Airflow does:
    # the project root and the dags folder on sys.path.
    for path in (dags_dir, project_root):
        if path and path not in sys.path:
            sys.path.insert(0, path)

    result = {"schema_version": 1, "dags": [], "import_errors": [], "files": []}

    logging.disable(logging.CRITICAL)
    try:
        install_parse_monkeypatches()
        from airflow.models.dagbag import DagBag

        dagbag = DagBag(dag_folder=dags_dir, include_examples=False)
    except Exception as exc:
        result["fatal"] = "{}: {}".format(type(exc).__name__, exc)
        emit(result)
        return

    def rel(path):
        try:
            return os.path.relpath(path, project_root)
        except (ValueError, TypeError):
            return path

    for path, message in dagbag.import_errors.items():
        result["import_errors"].append({"file": rel(path), "message": str(message).strip()})

    for dag_id, dag in dagbag.dags.items():
        result["dags"].append({"dag_id": dag_id, "file": rel(getattr(dag, "fileloc", ""))})

    for stat in getattr(dagbag, "dagbag_stats", None) or []:
        result["files"].append(
            {
                "file": rel(getattr(stat, "file", "")),
                "parse_time_seconds": parse_seconds(stat),
                "dag_ids": file_dag_ids(stat),
            }
        )

    emit(result)


if __name__ == "__main__":
    main()
