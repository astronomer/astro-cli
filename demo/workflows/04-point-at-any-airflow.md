# 4. Point at any Airflow

**The story.** One project names every Airflow it talks to — your laptop, two
Astro Deployments, an MWAA environment, a Composer environment, a bare URL —
and holds not one credential. One rule decides which of them a command acts
on, and your own machine is deliberately not on that rule.

**Shows:** deployment resolution and `astro use`, the query commands at the
top level and under `astro local`, the version-adaptive Airflow client, the
link kinds and the auth axis, the cloud auth resolvers, the deploy prompt.

**Needs:** the `astro` v2 binary, and a second local Airflow standing in for a
remote one — the setup is below. The MWAA and Composer calls need those
environments; that subsection says so where it starts.

Design of record: `docs/instances.md`.

---

## Set up something to talk to

The demo project links five Airflows and every one of them is a placeholder or
a cloud bill. So give it a sixth that answers: a second copy of the project,
running on this machine, reached the way any remote Airflow is reached — over
HTTP, with a token minted at request time.

```sh
cp -R demo/project /tmp/sandbox-airflow
cd /tmp/sandbox-airflow
astro local env variable set WAREHOUSE_URI --value sqlite:///include/warehouse.db --project
astro local env connection set warehouse --value sqlite:///include/warehouse.db --project
astro local env variable set ORDERS_API_URL --value https://catalog.example.com/products --project
astro local start          # prints `direct: http://localhost:12282`
```

A copy of the project is a clone, so it wants its three values before it will
start. That gate is workflow 2.

Add that address to `demo/project/pyproject.toml` as an endpoint link,
alongside the five that ship with it:

```toml
[tool.astro.deployments.sandbox]
url = 'http://localhost:12282'
auth = { method = 'airflow-token', username-env = 'SANDBOX_AIRFLOW_USER', password-env = 'SANDBOX_AIRFLOW_PASSWORD' }
```

```sh
export SANDBOX_AIRFLOW_USER=admin SANDBOX_AIRFLOW_PASSWORD=admin
```

`airflow-token` means "exchange these credentials at the Airflow's own
`/auth/token`". A local Airflow 3 runs the simple auth manager with every
caller an admin, so it mints for whoever asks and the pair can be anything —
but the CLI does not know that and does not care. It does what it does for a
self-hosted Airflow behind FAB or Keycloak: mint, hold the token in memory for
the run, send it. Nothing lands on disk.

Then start the demo project's own Airflow too, so both worlds are live:

```sh
cd demo/project
astro local start          # prints `direct: http://localhost:14582`
```

Ports differ every run. Everything below came from that pair.

## Your machine is not a deployment

```sh
astro local af dags list
```

```
→ local (http://localhost:14582)
DAG_ID           PAUSED  SCHEDULE                       OWNERS   TAGS         NEXT_RUN
orders_api_sync  yes     Never, external triggers only  airflow  demo,orders  -
orders_ingest    yes     0 0 * * *                      airflow  demo,orders  2026-08-03T00:00:00Z
orders_report    yes     Asset                          airflow  demo,orders  -
```

The Airflow on your laptop has its own spelling — `astro local af dags list`,
`astro local af runs list`, `astro local af health`, `astro local api /dags` — with
the target fixed to whatever `astro local start` is running. It is not a
deployment, it sits on no resolution ladder, and no flag, variable, or
`astro use` selection can move it.

`astro local api` takes what the standalone `af api` takes, so an `af api` line
ports by changing its first two words: `-F` typed fields (numbers, `true`,
`false`, `null`, `@file`), `--raw-field` string fields (af's `-f`), `-H`
headers, `-i` for the status line and headers (`-o json` for af's
`{status_code, headers, body}` object), `--raw` for an unversioned path, and the
two listings read from the running Airflow itself:

```sh
astro local api dags -F limit=10 -F only_active=true
astro local api variables -X POST -F key=port --raw-field value=8080
astro local api ls --filter variable
astro local api spec | jq '.paths | keys'
```

`astro api airflow` has the same `ls --filter` and `spec`, over the published
spec for the version its target reports.

That is the whole safety property: a top-level command can never quietly hit
localhost, and an `astro local` command can never hit prod. The two surfaces
are the same commands with the same flags and the same JSON. Only the target
differs.

## The committed inventory

At a terminal, `astro use` asks which link to act on, with the current one
highlighted and, when you did not select it yourself, labeled with what did:

```sh
astro use
```

```
Select the Deployment this project uses
 #     NAME              KIND         DEPLOYMENT, ENVIRONMENT OR URL
 1     dev               astro        your-dev-deployment-id
 2     prod              astro        your-prod-deployment-id                 ← default = true
 3     prod-composer     composer     orders-demo-composer
 4     prod-mwaa         mwaa         orders-demo-mwaa
 5     sandbox           endpoint     http://localhost:12282
 6     staging           endpoint     https://airflow.staging.example.com

>
```

Piped, or with `--output json`, it prints the same list without the question,
with a `*` on the current row:

```sh
astro use | cat
```

```
   NAME           KIND      DEPLOYMENT, ENVIRONMENT OR URL
   dev            astro     your-dev-deployment-id
*  prod           astro     your-prod-deployment-id              ← default = true
   prod-composer  composer  orders-demo-composer
   prod-mwaa      mwaa      orders-demo-mwaa
   sandbox        endpoint  http://localhost:12282
   staging        endpoint  https://airflow.staging.example.com
```

Naming the sandbox in the manifest is what turns a port you have to remember
into a word. The Airflow on your laptop is not in the list: it is not a
deployment, and `astro local` is how you reach it.

Nothing in that list was fetched. Coordinates print exactly as the manifest
writes them, so it works with the network off.

## One rule, every command

```
--deployment/-d  >  ASTRO_DEPLOYMENT  >  your selection (astro use)  >  manifest default link
```

- The flag wins, so CI needs no state.
- `ASTRO_DEPLOYMENT` is the ephemeral layer — the `AWS_PROFILE` / `KUBECONFIG`
  pattern. `export ASTRO_DEPLOYMENT=prod` makes one terminal the prod terminal.
- `astro use prod` is the durable per-project selection, written to per-user state
  outside the repo. Per-project, so switching for one repo cannot redirect
  another — the classic "wrong cluster" failure, gone by construction.
- The manifest's `default = true` link is the floor: a fresh clone with nothing
  selected acts on the team's default deployment.

Select the sandbox and the rule shifts under every command at once:

```sh
astro use sandbox
```

```
→ sandbox (endpoint http://localhost:12282)
this project now uses sandbox, for you only (astro use --unset to clear)
```

```sh
astro af dags list
```

```
→ sandbox (endpoint http://localhost:12282)
DAG_ID           PAUSED  SCHEDULE                       OWNERS   TAGS         NEXT_RUN
orders_api_sync  yes     Never, external triggers only  airflow  demo,orders  -
orders_ingest    yes     0 0 * * *                      airflow  demo,orders  2026-08-03T00:00:00Z
orders_report    yes     Asset                          airflow  demo,orders  -
```

Every command prints what it resolved to on stderr before it acts, so the
target is never invisible and stdout stays clean for `--output json`.

The other two layers reach the same Airflow without touching your selection:

```sh
astro af dags list -d sandbox
ASTRO_DEPLOYMENT=sandbox astro af dags list
```

Both print the same `→ sandbox (endpoint http://localhost:12282)` and the same
table.

Writes go the same way. Unpause a DAG through your selection, then look at both
Airflows:

```sh
astro af dags unpause orders_ingest
```

```
→ sandbox (endpoint http://localhost:12282)
unpaused orders_ingest
```

```sh
astro af dags list          # the sandbox
```

```
→ sandbox (endpoint http://localhost:12282)
DAG_ID           PAUSED  SCHEDULE                       OWNERS   TAGS         NEXT_RUN
orders_api_sync  yes     Never, external triggers only  airflow  demo,orders  -
orders_ingest    no      0 0 * * *                      airflow  demo,orders  2026-08-03T00:00:00Z
orders_report    yes     Asset                          airflow  demo,orders  -
```

```sh
astro local af dags list    # this machine
```

```
→ local (http://localhost:14582)
DAG_ID           PAUSED  SCHEDULE                       OWNERS   TAGS         NEXT_RUN
orders_api_sync  yes     Never, external triggers only  airflow  demo,orders  -
orders_ingest    yes     0 0 * * *                      airflow  demo,orders  2026-08-03T00:00:00Z
orders_report    yes     Asset                          airflow  demo,orders  -
```

One `PAUSED` cell moved, and it moved on the far Airflow rather than the near
one. That is the split, in one column.

## When a run fails

Reading a table is the easy half. The Airflow you are pointed at is also where
you go when something breaks, and that is three commands on the same rule.

Point the sandbox at a catalog endpoint that is not reachable — the setup above
used `catalog.example.com`, which the DAG recognizes as a placeholder and routes
around — then trigger it:

```sh
cd /tmp/sandbox-airflow
astro local env variable set ORDERS_API_URL --value https://catalog.internal.acme/products --project
astro local restart          # the value reaches tasks at start, so it needs one
cd -                         # back to demo/project
astro af runs trigger orders_api_sync -d sandbox
```

```
→ sandbox (endpoint http://localhost:12282)
triggered orders_api_sync run manual__2026-08-28T19:43:11.713309+00:00 (queued)
```

```sh
astro af runs list --dag-id orders_api_sync -d sandbox
```

```
→ sandbox (endpoint http://localhost:12282)
DAG_ID           RUN_ID                                    STATE   TYPE    LOGICAL_DATE  START                 DURATION
orders_api_sync  manual__2026-08-28T19:43:11.713309+00:00  failed  manual  -             2026-08-28T19:43:12Z  5s
```

Failed — but a run is several tasks and that says nothing about which one:

```sh
astro af runs tasks orders_api_sync manual__2026-08-28T19:43:11.713309+00:00 -d sandbox
```

```
→ sandbox (endpoint http://localhost:12282)
TASK_ID           STATE            TRY  START                 DURATION
pick_source       success          1    2026-08-28T19:43:14Z  2s
fetch_from_api    failed           1    2026-08-28T19:43:15Z  81ms
use_local_sample  skipped          0    2026-08-28T19:43:15Z  -
summarize         upstream_failed  0    2026-08-28T19:43:16Z  -
```

The whole shape of the run in four rows: the branch chose the API, the sample
path was skipped, the fetch failed, and the summary never ran. `skipped` and
`upstream_failed` are different words for different reasons, and neither is the
fault.

`fetch_from_api` is the one that broke, so read its log:

```sh
astro af tasks logs orders_api_sync manual__2026-08-28T19:43:11.713309+00:00 fetch_from_api -d sandbox
```

```
→ sandbox (endpoint http://localhost:12282)
::group::Log message source details
::endgroup::
DAG bundles loaded: dags-folder
Filling up the DagBag from /…/sandbox-airflow/dags/orders_api_sync.py
Task failed with exception
Traceback (most recent call last):
  File "/…/python3.13/urllib/request.py", line 1319, in do_open
  …
  File "/…/python3.13/socket.py", line 977, in getaddrinfo
gaierror: [Errno 8] nodename nor servname provided, or not known

During handling of the above exception, another exception occurred:

Traceback (most recent call last):
  …
  File "/…/python3.13/site-packages/airflow/sdk/execution_time/callback_runner.py", line 82, in run
  File "/…/sandbox-airflow/dags/orders_api_sync.py", line 48, in fetch_from_api
  File "/…/python3.13/urllib/request.py", line 189, in urlopen
  …
URLError: <urlopen error [Errno 8] nodename nor servname provided, or not known>
```

The host does not resolve, at `orders_api_sync.py` line 48. Chained exceptions
print the way Python prints them — oldest first, the one that actually killed
the task last — because Airflow 3 serializes the whole chain as data and this
renders it rather than showing you the summary line and stopping.

Airflow 2 gets there differently and lands in the same place: its log endpoint
serves rendered text, so the traceback arrives already formatted, carets and
source lines included. Neither generation makes you open the UI to find out what
went wrong.

Two commands fold those steps together. `runs trigger-wait` starts a run and
waits for it, then lists the failed and `upstream_failed` tasks if it did not
succeed. It exits 0 on success, 1 on a failed run and 2 when `--timeout` runs
out first. `runs diagnose` reads a finished run, every task instance in it, and
the counts by state:

```sh
astro af runs trigger-wait orders_api_sync --timeout 300 -d sandbox
astro af runs diagnose orders_api_sync manual__2026-08-28T19:43:11.713309+00:00 -d sandbox
```

Both print the `tasks logs` command for the task that failed. The rest of the
surface ported from the standalone `af`: `dags errors` and `dags warnings` (the
import errors and warnings `health` summarizes), `dags explore` (a DAG, its
tasks and its source in one read), `assets triggers` (the asset events that
started a run), and `version`, `providers`, `plugins` and `config` for the
Airflow itself. Every one is also under `astro local af`.

## An Airflow nobody declared

`--url` skips the rule entirely, for an Airflow no project links — a
colleague's dev server, something a platform team stood up this morning. It
needs no project and writes nothing:

```sh
export ASTRO_AIRFLOW_TOKEN=$(curl -s -X POST http://localhost:12282/auth/token \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}' | jq -r .access_token)

astro af dags list --url http://localhost:12282
```

```
→ http://localhost:12282
DAG_ID           PAUSED  SCHEDULE                       OWNERS   TAGS         NEXT_RUN
orders_api_sync  yes     Never, external triggers only  airflow  demo,orders  -
orders_ingest    no      0 0 * * *                      airflow  demo,orders  2026-08-03T00:00:00Z
orders_report    yes     Asset                          airflow  demo,orders  -
```

A bare URL says nothing about how its Airflow checks callers, so the credential
comes from the environment — `ASTRO_AIRFLOW_TOKEN`, or
`ASTRO_AIRFLOW_USERNAME` and `ASTRO_AIRFLOW_PASSWORD` together. A username and
password are exchanged at the Airflow's own `/auth/token` first, so the curl
above is only needed for a token you already hold: an Airflow 3 gets the JWT it
mints, and an Airflow 2, which serves no `/auth/token`, gets basic auth. Set none and
nothing is sent, because an open dev server is a real case and a guessed
credential would only turn a clear 401 into a confusing one:

```
→ http://localhost:12282
Error: GET /dags: airflow returned 401 Unauthorized: Not authenticated
```

## When the rule finds nothing

A project that links nothing is told both spellings, because whoever hits this
is one command away from either:

```sh
astro af dags list          # in a fresh `astro init` project
```

```
Error: no deployment to act on: this project links none ([tool.astro.deployments] in pyproject.toml). To reach an Airflow no project declares, pass --url. The Airflow on this machine is not a deployment and never resolves here; it has its own commands, under `astro local`.
For this machine: `astro local af dags`
```

Several links with no default is a different failure — there is something to
act on, nobody said which — so it names every way to say which. Delete
`default = true` from the demo project's `prod` link to see it:

```
Error: several deployments are linked and none is the default (dev, prod, prod-composer, prod-mwaa, sandbox, staging): pick one with -d <name>, export ASTRO_DEPLOYMENT=<name>, or select one with `astro use <name>`
For this machine: `astro local af dags`
```

That is what a script gets. At a terminal nobody is asked twice: the command
asks once, and the answer becomes your selection.

```
Which deployment should this project use?
 #     NAME
 1     dev
 2     prod
 3     prod-composer
 4     prod-mwaa
 5     sandbox
 6     staging

> sandbox
picked sandbox — this project uses it from now on, for you only (astro use --unset to clear)
→ sandbox (endpoint http://localhost:12282)
DAG_ID           PAUSED  SCHEDULE                       OWNERS   TAGS         NEXT_RUN
…
```

A name no link declares lists the ones that do, and the machine's own name is
refused as a name with a new home rather than as a typo:

```sh
astro af dags list -d nope
```

```
Error: no deployment named "nope"; known deployments: dev, prod, prod-composer, prod-mwaa, sandbox, staging
```

```sh
astro af dags list -d local
```

```
Error: `local` is not a deployment: it is this machine, and it has its own commands — `astro local start` runs it, `astro local af dags list` and `astro local af health` read it. `astro use` selects among deployments only
```

## Auth is its own axis

A link's kind says where an Airflow is. It says nothing about how that Airflow
checks callers, so auth is a table of its own:

| method | credential, resolved at request time |
| --- | --- |
| `astro` | the current session, or `ASTRO_API_TOKEN` |
| `aws` | the AWS credential chain → `InvokeRestApi`, falling back to the web-login-token exchange |
| `google` | Application Default Credentials → an OAuth2 access token |
| `basic` | username and password from env vars |
| `token` | a static bearer from an env var |
| `airflow-token` | credentials exchanged at the Airflow's own `/auth/token` (FAB, SimpleAuthManager, Keycloak) |
| `exec` | run a command, read a token — the kubectl / git-credential-helper pattern. `command` is argv, `['acme-token', '--profile', 'prod']`, run directly and never through a shell, so there are no quoting rules to get wrong |
| `none` | local instances, open dev servers |

The crosses are the point. A self-hosted Airflow behind Google IAP is a `url`
link with `method = 'google'`. An Airflow 3 under Keycloak is `url` +
`airflow-token`. Whatever your platform team ships is `url` + `exec`. No
credential lands on disk that was not already there.

A link that says nothing takes its kind's default — Astro links `astro`, MWAA
`aws`, Composer `google` — and every method that reads a credential reads it
from an env var it names, never from a literal in the manifest. A name the
machine has no value for is the clone-and-run gate from workflow 2 doing its
job on a new kind of value, one mechanism rather than a second one:

```sh
astro af dags list -d staging
```

```
→ staging (endpoint https://airflow.staging.example.com)
Error: deployment "staging" needs the env var STAGING_AIRFLOW_TOKEN, which is not set on this machine.
      provide it:  astro local env variable set STAGING_AIRFLOW_TOKEN --project
```

## The two managed platforms

> **Not captured.** The two blocks in this subsection are the only ones on this
> page that were not run: they need a live MWAA or Composer environment, and
> the demo stands neither up. Both commands were typed, and the `→` line each
> prints is real; the table under it is written from the spec. Read only these
> two that way.

```sh
astro af dags list -d prod-mwaa
```

```
→ prod-mwaa (mwaa environment orders-demo-mwaa)
[table of DAGs]
```

No `af`, no URL, no stored token. The link was already in `pyproject.toml`; the
`aws` resolver turned your ordinary AWS credentials into a signed call through
MWAA's `InvokeRestApi`, at request time, with nothing written to disk. Composer
is the same shape through Application Default Credentials:

```sh
astro af dags list -d prod-composer
```

```
→ prod-composer (composer environment orders-demo-composer)
[table of DAGs]
```

Both take the rest of their coordinates — region, project, location — from
`[tool.astro.targets.*]`: the link names the environment, the target table says
where that environment lives.

## Deploy asks, always

The same rule reaches the deploy path with one deliberate difference. Shipping
code is too consequential to decide from a marker, so `astro deploy` never
picks for you — the layers move the cursor and do nothing more.

```sh
astro deploy
```

```
Deploy to which deployment?
 #     NAME     WHERE                                        PRESELECTED BY
 1     dev      astro deployment your-dev-deployment-id
 2     prod     astro deployment your-prod-deployment-id     default = true

> [2]
```

The label names what moved the cursor, because three different things can.
Export a variable and the same prompt reads differently:

```sh
ASTRO_DEPLOYMENT=dev astro deploy
```

```
Deploy to which deployment?
 #     NAME     WHERE                                        PRESELECTED BY
 1     dev      astro deployment your-dev-deployment-id      ASTRO_DEPLOYMENT
 2     prod     astro deployment your-prod-deployment-id

> [1]
```

`astro use` is the third. Answer the prompt on a logged-in machine and the
build starts, and stops at the shipped placeholder ids — which is where this
walkthrough stops too:

```
→ prod (astro deployment your-prod-deployment-id)
Building your project image, this can take a few minutes...
Error: deployment with id your-prod-deployment-id not found
```

In CI, where nobody is there to answer, naming the deployment is required — an
`astro use` selection or an exported variable never decides a deploy:

```
Error: a deploy must name the deployment it ships to: `astro deploy <name>` or --deployment <name>. Deploy never picks for you — `astro use`, ASTRO_DEPLOYMENT, or `default = true` only preselect the prompt. Deployable links: dev, prod
```

Only Astro links are offered, and a link of another kind is turned away by name
and by kind:

```sh
astro deploy prod-mwaa
```

```
Error: link "prod-mwaa" is mwaa, and astro deploy ships to Astro Deployments, astro links: dev, prod
```

What a deploy does after the prompt is [workflow 5](05-three-clouds.md).

## What this replaced, and what is left

The CLI used to have four answers to "what am I pointed at": login contexts
with a mutable workspace inside, 1.x per-project pins, the manifest's links,
and `af`'s separate named-instance inventory. Two questions with one home each
replace all four — a session is who you are, a deployment is what you are
acting on, and a deployment lives in the manifest, committed, in front of you.
The reasoning is in `docs/instances.md`.

What is left is the writing half. `astro deploy` shipping to a non-Astro link
is the last mile: package, then sync, then update the environment, under your
own AWS or Google credentials, from the target config already in the manifest.
`astro package mwaa` and `astro package composer` build the right artifact
today and print the exact upload command; running it is still yours.
Deploying to MWAA and Composer is not yet supported.
