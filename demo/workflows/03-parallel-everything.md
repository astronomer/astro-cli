# 3. Parallel everything

**The story.** Three Airflows at once on one laptop — two projects and a git
worktree — each on its own stable hostname, none of them fighting over port
8080. Then one command shows you all of them.

**Shows:** the local proxy and per-project hostnames, worktree-aware naming,
`astro local list` and its stale sweep, the per-project state records.

**Needs:** the `astro` v2 binary, Python, git. About 500 MB of disk for three
virtualenvs (uv shares its wheel cache, so the second and third take seconds).

There is a script for the setup: [`bin/parallel-demo.sh`](bin/parallel-demo.sh).

---

## Two projects

```sh
cp -R demo/project /tmp/orders-demo
cd /tmp/orders-demo
astro local env variable set WAREHOUSE_URI --value 'sqlite:///include/warehouse.db'
astro local env connection set warehouse --value 'sqlite:///include/warehouse.db'
astro local env variable set ORDERS_API_URL --value 'https://catalog.example.com/products'
astro local start
```

```
url: http://orders-demo.localhost:6563
direct: http://localhost:18897
```

Now a second one, same code, different directory:

```sh
cp -R demo/project /tmp/orders-experiment
cd /tmp/orders-experiment
# same three env set commands
astro local start
```

```
url: http://orders-experiment.localhost:6563
direct: http://localhost:11411
```

Two Airflows, two hostnames, two ports — and both hostnames answer on the
same proxy port, 6563. You never chose a port and never hit a conflict.

## And a git worktree

```sh
cd /tmp/orders-demo && git init -q && git add -A && git commit -qm init
git worktree add ../orders-wt -b feature-x
cd ../orders-wt
# same three env set commands
astro local start
```

```
url: http://orders-wt.orders-demo.localhost:6563
direct: http://localhost:19371
```

A worktree gets `<worktree>.<repo>.localhost`, so two branches of the same
repo never collide — the branch you are on is in the URL. That is the point
for anyone reviewing a PR while their own branch keeps running.

## See all of them

```sh
astro local list
```

```
PROJECT                  HOSTNAME                         MODE        STATE    PORT   UPTIME
/tmp/orders-demo         orders-demo.localhost            standalone  running  14751  32s
/tmp/orders-wt           orders-wt.orders-demo.localhost  standalone  running  19371  11s
/tmp/orders-experiment   orders-experiment.localhost      standalone  running  10129  22s
```

This is machine-wide, not project-wide: it reads every state record under the
cache root, whatever directory you run it from. Each row is checked through
its own engine — a standalone record checks its process group, a docker record
asks the container engine.

```sh
astro local list --output json
```

```json
{"projects":[{"project":"/tmp/orders-demo","hostname":"orders-demo.localhost","mode":"standalone","state":"running","port":14751,"url":"http://localhost:14751","started_at":"2026-07-30T20:34:22Z","uptime":"32s"}]}
```

One object with the rows under `projects` — `[]` when nothing is running — the
same convention every list in v2 uses. NDJSON is for streams, such as logs.

## Prove the routing

```sh
curl -s -o /dev/null -w '%{http_code}\n' http://orders-demo.localhost:6563/
curl -s -o /dev/null -w '%{http_code}\n' http://orders-experiment.localhost:6563/
curl -s -o /dev/null -w '%{http_code}\n' http://orders-wt.orders-demo.localhost:6563/
```

```
200
200
200
```

Three names, one port, three different Airflows. `.localhost` resolves to the
loopback address without any hosts-file edit, so this works on a fresh machine
with nothing configured.

## Clean up

```sh
astro local list --all      # includes records whose Airflow is gone
astro local list --clean    # remove the dead ones and their routes
```

`--clean` is the machine-wide sweep, and it lives on `list` rather than
`reset` because `reset` acts on one project while this spans every project and
worktree you have ever started.

Stop the live ones from their own directories:

```sh
cd /tmp/orders-demo       && astro local stop
cd /tmp/orders-experiment && astro local stop
cd /tmp/orders-wt         && astro local stop
```

> **A detail worth knowing.** The hostname comes from the **directory** (or
> the worktree and repo names), not from `[project] name` in the manifest.
> Copy a project to a new folder and the hostname follows the folder. Identity
> is the project path, hashed; hostnames are display labels and two projects
> are allowed to want the same one.
