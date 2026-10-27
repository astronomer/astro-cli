# Which Airflow a command talks to

A project can talk to more than one Airflow: the one running on your machine, and the Deployments, MWAA and Composer environments, and self-hosted instances its manifest links. This page describes how each command picks one. The links themselves are described in the [manifest reference](manifest-reference.md#toolastrodeploymentsname); the resolution code is `Select` in [`pkg/instances`](../pkg/instances/resolve.go).

## The commands

| command | talks to | target flags |
| --- | --- | --- |
| `astro local af <family>` | this project's running local Airflow | none |
| `astro local api <endpoint>` | this project's running local Airflow | none |
| `astro af <family>` (alias `astro airflow`) | a linked Airflow | `-d/--deployment`, `--url` |
| `astro api airflow <endpoint>` | a linked Airflow | `-d/--deployment`, `--url` (required) |
| `astro deploy` | an Astro Deployment | positional or `--deployment` ([its own rule](#deploy-always-asks)) |

The `af` families are `dags`, `runs`, `tasks`, `assets`, `connections`, `variables`, `pools`, `health`, `version`, `providers`, `plugins` and `config`. `astro local af` and `astro af` run the same code; only the target differs.

**Your machine is reached only through `astro local`.** `local` is a reserved link name: no link may take it, and naming it at any layer below is refused (`ErrLocalNotADeployment`). `astro local af` with nothing running fails with "start one with `astro local start`".

## Resolution order for `astro af`

The first of these that names something wins:

1. **`--url <address>`**: an Airflow addressed directly, with no link. It cannot be combined with `-d` and works outside a project.
2. **`-d/--deployment <name>`**: a link name, else an Astro Deployment id or name (see [A Deployment no link declares](#a-deployment-no-link-declares)).
3. **`ASTRO_DEPLOYMENT`**: a link name, from the environment (not an id or a Deployment name).
4. **Your selection**, set with `astro use` (below).
5. **The default link**: the link marked `default = true`, or a project's only link. Adding a second link to a project whose only link is unmarked (`astro link add`, a deploy that links what it picked, Astro Desktop) marks that first link `default = true`, so the project keeps pointing where it did, and says so. A link written `default = false` (by hand, or by `astro link default --unset`) is never marked. Nothing else picks a default: a project that already has several unmarked links stays without one until you run `astro link default NAME`.

With nothing resolved:

- a project with no links fails (`ErrNone`);
- a project with several links and no default fails, telling you to pick with `-d`, `ASTRO_DEPLOYMENT` or `astro use`. An interactive run without `--output json` asks once instead, and saves the answer as your selection.

Both errors add `For this machine: astro local af <family>`. Every run prints the target it resolved on stderr, as `→ name (kind where)`.

`astro api airflow` takes only steps 1 and 2. With neither it fails and lists `-d <link>`, `--url` and `astro local api`. Its `ls` and `spec` subcommands need no target.

## A Deployment no link declares

`astro af -d` and `astro api airflow -d` read a value no link declares the same way, which is the way `--deployment` reads on `astro deployment`:

- a Deployment id (`c` and 24 lowercase letters or digits) is that Deployment, fetched by id across the organization, with no other call. When a link carries that id, the link answers for it, with its name and auth;
- anything else is a Deployment's name, looked up in the project's `[tool.astro] workspace`, else the current workspace of the login the command uses (for `astro af`, the login for the project's `domain`; for `astro api airflow`, the current context). The project's workspace is listed in the project's `[tool.astro] organization` when it names one, else in the login's. No Deployment of that name, or several (listed by id), is an error. A Deployment a link carries is that link, with its name and auth. `astro api airflow` reads through the current context, so in a project whose `domain` is another Astro host it refuses a name up front, naming that host, with `astro context switch <host>` or the Deployment id as the way on.

Inside a project, a name that matches neither a link nor a Deployment is most likely a mistyped link, so the error leads with the project's links. Neither command takes a workspace flag. On `astro api airflow`, 1.x's `--workspace-id` (`-W`) is still accepted, hidden, and does nothing, as in 1.x. `-O/--organization-id` picks the organization an id is fetched in. For a name it has to be the organization of the workspace being searched (the project's `[tool.astro] organization` when the project names its workspace and an organization, else the context's); any other organization is refused, with a request to pass the Deployment id.

## `astro use`

`astro use NAME` makes a link your selection for this project. It checks the name against the manifest's links, and warns when `ASTRO_DEPLOYMENT` is set and will override it. `astro use --unset` clears it. Bare `astro use` opens a picker at a terminal (with an option to clear the selection). Off a terminal, or with `--output json`, it reports the link commands act on now (`current`, the name) and what made it so (`from`: `env`, `selection` or `default`), then each link as a row: `name`, `kind`, `where`, `url`, `auth_method`, and two booleans, `current` (this row is the one) and `default` (this row is the project's default link). Both mark rows in the words the `astro link` pickers and the deploy prompt use: `default` on the project's default link, `from ASTRO_DEPLOYMENT` on a row the variable made current, and `default, from ASTRO_DEPLOYMENT` when both fall on one row. Your own selection carries no label; the highlight (or `*`) says it.

The selection is per user and per project, never committed: `state.json` in the project's cache directory (`$XDG_CACHE_HOME/astro/projects/<project id>/`, else `~/.cache/astro/…`), mode `0600`. The field is `instance`; `deployment` is also read and written, for compatibility with pre-release builds.

## Deploy always asks

`astro deploy` never resolves from ambient state. Shipping code is too consequential to decide from a selection, an exported variable or a marker in a file nobody looked at. The target is named on the command line, or an interactive run asks. Only `default = true` (or a project's only link) preselects the entry in that prompt. `ASTRO_DEPLOYMENT` and your selection play no part in it: they choose which Airflow the query commands read, and where code ships is the project's, so the CLI and Astro Desktop highlight the same entry. A run that cannot be asked and names nothing fails. Details are in [deploy.md](deploy.md#3-selecting-the-deployment).

## Link kinds and authentication

A link's kind follows from its fields: `url` makes an **endpoint** link; `target = 'mwaa'` or `'composer'` (with `environment`) an **MWAA** or **Composer** link; otherwise (with `deployment`) an **Astro** link.

Where the Airflow is comes per kind: an Astro Deployment's address from the control plane (`internal/instancelocate`), a Composer environment's from Google's API (`pkg/instancelocate`), an MWAA environment through `pkg/awsauth`, and an endpoint from its `url`.

How the CLI authenticates is the link's `auth.method`, defaulting by kind (Astro `astro`, MWAA `aws`, Composer `google`; an endpoint link must name one):

| method | credential |
| --- | --- |
| `astro` | `ASTRO_API_TOKEN`, else your login for the project's [`domain`](manifest-reference.md#domain) |
| `aws` | the AWS credential chain, through MWAA's `InvokeRestApi`, falling back to the web-login-token exchange (`pkg/awsauth`) |
| `google` | Application Default Credentials (`pkg/googleauth`) |
| `basic` | username and password from the env vars the link names |
| `token` | a bearer token from the env var the link names |
| `airflow-token` | credentials exchanged at the Airflow's own `/auth/token` |
| `exec` | a token printed by a command the link names |
| `none` | nothing |

The manifest only ever names environment variables, never a credential. A `--url` target reads `ASTRO_AIRFLOW_TOKEN`, or `ASTRO_AIRFLOW_USERNAME` and `ASTRO_AIRFLOW_PASSWORD` (exchanged at `/auth/token`, falling back to basic auth), and sends nothing when none is set.

`pkg/awsauth` and `pkg/googleauth` are separate modules, so a consumer of `pkg/instances` links neither cloud SDK unless it adopts that door (see [architecture.md](architecture.md#sub-module-rules)).

## When a Deployment's Airflow does not answer

For an Astro link, a 502, 503 or 504 from the Airflow is explained by asking the control plane why, and the failure carries one of these `kind`s in `--output json` (see [Output](architecture.md#output)):

| kind | meaning |
| --- | --- |
| `deployment_hibernating` | the Deployment is hibernating |
| `deployment_deploying` | a deploy is rolling out; try again |
| `deployment_unhealthy` | the Deployment reports unhealthy |
| `airflow_unavailable` | Astro reports it healthy but Airflow did not answer; try again |

## Editing links

`astro link add`, `astro link remove` and `astro link default [--unset]` edit the project's links in `pyproject.toml`, and `astro link workspace [--unset]` its workspace link. They are described in [Linking from the command line](manifest-reference.md#linking-from-the-command-line).
