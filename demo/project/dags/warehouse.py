"""Shared helper: resolve WAREHOUSE_URI to a file on disk.

Not a pipeline — a plain module both pipelines import. The demo warehouse is
SQLite so the whole project runs offline with nothing installed. Point
WAREHOUSE_URI at anything else and this says so plainly rather than
pretending.
"""

from __future__ import annotations

import os
from pathlib import Path

# The project root: this file lives one directory below it. Relative SQLite
# paths in WAREHOUSE_URI resolve against this, so nothing depends on which
# directory a task happens to run in.
PROJECT_ROOT = Path(__file__).resolve().parent.parent


def warehouse_path() -> Path:
    uri = os.environ["WAREHOUSE_URI"]
    if not uri.startswith("sqlite:///"):
        raise ValueError(
            f"this demo writes SQLite only, and WAREHOUSE_URI is {uri!r}. "
            "Set it to something like sqlite:///include/warehouse.db"
        )
    path = Path(uri.removeprefix("sqlite:///"))
    if not path.is_absolute():
        path = PROJECT_ROOT / path
    path.parent.mkdir(parents=True, exist_ok=True)
    return path
