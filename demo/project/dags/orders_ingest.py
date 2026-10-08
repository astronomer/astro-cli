"""Load a day of orders into the warehouse.

Reads three declared values from `[tool.astro.env]` in pyproject.toml:

- ``WAREHOUSE_URI``  — required, no default. This is the clone-and-run gate:
  a fresh clone refuses to start until you set it.
- ``batch_size``     — an Airflow Variable with a committed default of 500.
- ``LOG_LEVEL``      — a plain env var with a committed default of ``info``.
"""

from __future__ import annotations

import logging
import os
import random
import sqlite3

from airflow.sdk import Asset, Variable, dag, task
from warehouse import warehouse_path

log = logging.getLogger(__name__)

ORDERS = Asset("warehouse://orders")

REGIONS = ["emea", "amer", "apac"]
PRODUCTS = ["widget", "sprocket", "gizmo", "doohickey"]

CREATE_ORDERS = """
CREATE TABLE IF NOT EXISTS orders (
    order_id     TEXT PRIMARY KEY,
    day          TEXT NOT NULL,
    region       TEXT NOT NULL,
    product      TEXT NOT NULL,
    amount_cents INTEGER NOT NULL
)
"""

INSERT_ORDER = """
INSERT OR REPLACE INTO orders
VALUES (:order_id, :day, :region, :product, :amount_cents)
"""


@dag(schedule="@daily", catchup=False, tags=["orders", "demo"], doc_md=__doc__)
def orders_ingest():
    @task
    def generate(**context) -> list[dict]:
        """Make a deterministic day of orders, sized by the batch_size Variable."""
        level = os.environ.get("LOG_LEVEL", "info").upper()
        log.setLevel(getattr(logging, level, logging.INFO))

        size = int(Variable.get("batch_size", default="500"))
        day = context["logical_date"].date().isoformat()
        rng = random.Random(day)  # same day, same orders — reruns are stable

        orders = [
            {
                "order_id": f"{day}-{n:05d}",
                "day": day,
                "region": rng.choice(REGIONS),
                "product": rng.choice(PRODUCTS),
                "amount_cents": rng.randrange(500, 25_000),
            }
            for n in range(size)
        ]
        log.info("generated %d orders for %s", len(orders), day)
        return orders

    @task(outlets=[ORDERS])
    def load(orders: list[dict]) -> int:
        """Write the orders to the warehouse and report the row count."""
        path = warehouse_path()
        with sqlite3.connect(path) as conn:
            conn.execute(CREATE_ORDERS)
            conn.executemany(INSERT_ORDER, orders)
            total = conn.execute("SELECT count(*) FROM orders").fetchone()[0]

        log.info("loaded %d orders into %s (%d rows total)", len(orders), path, total)
        return total

    load(generate())


orders_ingest()
