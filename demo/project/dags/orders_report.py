"""Turn the warehouse into a revenue report.

Scheduled on the asset ``orders_ingest`` produces, so triggering the ingest
runs this one too — one command, two pipelines, visible output.

It also resolves the ``warehouse`` connection declared in
``[tool.astro.env.connections]``. That connection is required with no default,
so it is part of the clone-and-run gate alongside ``WAREHOUSE_URI``. This
prints the connection's type and host and never its password.
"""

from __future__ import annotations

import logging
import sqlite3

from airflow.sdk import Asset, dag, task
from warehouse import PROJECT_ROOT, warehouse_path

# Airflow 3.1 moved BaseHook onto the task SDK; 3.0 still keeps it at the old
# path. This project pins 3.1, but MWAA offers no 3.1 at all, so the same code
# has to parse on 3.0 as well — `astro local check --target mwaa` is what tells
# you that, before an upload rather than after.
try:
    from airflow.sdk import BaseHook
except ImportError:  # Airflow < 3.1
    from airflow.hooks.base import BaseHook

log = logging.getLogger(__name__)

ORDERS = Asset("warehouse://orders")
REPORT = PROJECT_ROOT / "include" / "out" / "daily_report.md"

REVENUE_BY_REGION = """
SELECT region, count(*) AS orders, sum(amount_cents) AS cents
FROM orders
GROUP BY region
ORDER BY cents DESC
"""


@dag(schedule=[ORDERS], catchup=False, tags=["orders", "demo"], doc_md=__doc__)
def orders_report():
    @task
    def describe_connection() -> str:
        """Show that the declared connection resolved, without leaking it."""
        conn = BaseHook.get_connection("warehouse")
        log.info(
            "connection 'warehouse' resolved: type=%s host=%s schema=%s",
            conn.conn_type,
            conn.host,
            conn.schema,
        )
        return conn.conn_type or "unknown"

    @task
    def revenue_by_region() -> list[tuple[str, int, int]]:
        with sqlite3.connect(warehouse_path()) as conn:
            rows = conn.execute(REVENUE_BY_REGION).fetchall()
        for region, orders, cents in rows:
            log.info("%-6s %6d orders  $%s", region, orders, f"{cents / 100:,.2f}")
        return rows

    @task
    def write_report(rows: list[tuple[str, int, int]], conn_type: str) -> str:
        """Write the report to include/out/ so there is something to open."""
        lines = [
            "# Orders — revenue by region",
            "",
            f"Warehouse connection type: `{conn_type}`",
            "",
            "| region | orders | revenue |",
            "| --- | ---: | ---: |",
        ]
        lines += [f"| {r} | {n} | ${c / 100:,.2f} |" for r, n, c in rows]
        REPORT.parent.mkdir(parents=True, exist_ok=True)
        REPORT.write_text("\n".join(lines) + "\n")
        log.info("wrote %s", REPORT)
        return str(REPORT)

    write_report(revenue_by_region(), describe_connection())


orders_report()
