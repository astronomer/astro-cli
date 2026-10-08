"""Pick a source for the product catalog, then load it.

The point of this one is ``ORDERS_API_URL``, declared as
``{ source = 'workspace' }``: it resolves from the workspace's Environment
Manager when you are logged in, and from the shell, the project ``.env``, or
``astro local env variable set`` otherwise. Nobody has to hand it to you.

The branch keeps the demo honest offline: unless ``ORDERS_API_URL`` names a
real host, the run takes the local-sample path and still finishes.
"""

from __future__ import annotations

import logging
import os

from airflow.sdk import dag, task

log = logging.getLogger(__name__)

PLACEHOLDER_HOSTS = ("example.com", "example.org", "localhost")

SAMPLE_CATALOG = [
    {"sku": "widget", "list_cents": 1_200},
    {"sku": "sprocket", "list_cents": 3_400},
    {"sku": "gizmo", "list_cents": 8_900},
    {"sku": "doohickey", "list_cents": 450},
]


@dag(schedule=None, catchup=False, tags=["orders", "demo"], doc_md=__doc__)
def orders_api_sync():
    @task.branch
    def pick_source() -> str:
        url = os.environ.get("ORDERS_API_URL", "")
        log.info("ORDERS_API_URL resolved to %r", url)
        if url and not any(host in url for host in PLACEHOLDER_HOSTS):
            return "fetch_from_api"
        log.info("no real catalog endpoint configured — using the bundled sample")
        return "use_local_sample"

    @task
    def fetch_from_api() -> list[dict]:
        import json
        import urllib.request

        url = os.environ["ORDERS_API_URL"]
        with urllib.request.urlopen(url, timeout=10) as resp:  # noqa: S310
            return json.load(resp)

    @task
    def use_local_sample() -> list[dict]:
        return SAMPLE_CATALOG

    @task(trigger_rule="none_failed_min_one_success")
    def summarize(from_api: list[dict], from_sample: list[dict]) -> int:
        catalog = from_api or from_sample
        for item in catalog:
            log.info("%-10s $%s", item["sku"], f"{item['list_cents'] / 100:,.2f}")
        log.info("catalog has %d products", len(catalog))
        return len(catalog)

    source = pick_source()
    api = fetch_from_api()
    sample = use_local_sample()
    source >> [api, sample]
    summarize(api, sample)


orders_api_sync()
