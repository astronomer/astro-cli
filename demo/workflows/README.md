# Workflows

Seven walkthroughs. Each one states the story in two sentences, then gives the
exact commands and the output they actually produced. Run them in order for a
full demo, or pick one.

| # | Workflow | Runs in | Needs |
| --- | --- | --- | --- |
| 1 | [Zero to Airflow in a minute](01-zero-to-airflow.md) | ~2 min | Python, uv |
| 2 | [The clone-and-run gate](02-clone-and-run-gate.md) | ~3 min | Python, uv |
| 3 | [Parallel everything](03-parallel-everything.md) | ~3 min | Python, uv, git |
| 4 | [Point at any Airflow](04-point-at-any-airflow.md) | ~5 min | Python, uv; a second local Airflow, set up in the workflow |
| 5 | [Ship the same project to three clouds](05-three-clouds.md) | ~5 min | Docker for the image paths; network for the MWAA constraints |
| 6 | [Agent-ready](06-agent-ready.md) | ~3 min | `jq` for the examples |
| 7 | [Otto in the loop](07-otto.md) | ~3 min | an Astro login; network |

Two of them have scripts:

```sh
./bin/parallel-demo.sh            # workflow 3: three Airflows at once
./bin/parallel-demo.sh --clean    # stop and remove them

./bin/package-all.sh              # workflow 5: pre-flight, then all three artifacts
./bin/package-all.sh --no-image   # skip the Docker step
```

Both take `ASTRO=/path/to/astro` if the v2 binary is not first on your PATH.

## Reading the output blocks

Every fenced block after a command is real output from a real run, trimmed
where a hundred lines of uv install would help nobody. Absolute paths are
shortened to `/…/`. Ports and pids differ every run — the CLI picks a free
port rather than fighting over 8080, which is workflow 3's whole point.

Where something does not work yet, the workflow says so at the point you would
hit it. Two blocks in workflow 4 — the MWAA and Composer query calls — are the
only ones written from the spec rather than captured, and they carry a note
saying so.
