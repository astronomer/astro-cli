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
2. **`-d/--deployment <name>`**: a link name, or an Astro Deployment id when no link has that name.
3. **`ASTRO_DEPLOYMENT`**: the same, from the environment.
4. **Your selection**, set with `astro use` (below).
5. **The default link**: the link marked `default = true`, or a project's only link.

With nothing resolved:

- a project with no links fails (`ErrNone`);
- a project with several links and no default fails, telling you to pick with `-d`, `ASTRO_DEPLOYMENT` or `astro use`. An interactive run without `--output json` asks once instead, and saves the answer as your selection.

Both errors add `For this machine: astro local af <family>`. Every run prints the target it resolved on stderr, as `→ name (kind where)`.

`astro api airflow` takes only steps 1 and 2. With neither it fails and lists `-d <link>`, `--url` and `astro local api`. Its `ls` and `spec` subcommands need no target.

## `astro use`

`astro use NAME` makes a link your selection for this project. It checks the name against the manifest's links, and warns when `ASTRO_DEPLOYMENT` is set and will override it. `astro use --unset` clears it. Bare `astro use` opens a picker at a terminal (with an option to clear the selection). Off a terminal, or with `--output json`, it lists the links with `current`, `from` (`env`, `selection` or `default`) and each link's `name`, `kind`, `where`, `url` and `auth_method`.

The selection is per user and per project, never committed: `state.json` in the project's cache directory (`$XDG_CACHE_HOME/astro/projects/<project id>/`, else `~/.cache/astro/…`), mode `0600`. The field is `instance`; `deployment` is also read and written, for compatibility with pre-release builds.

## Deploy always asks

`astro deploy` never resolves from ambient state. Shipping code is too consequential to decide from a selection, an exported variable or a marker in a file nobody looked at. The target is named on the command line, or an interactive run asks. `ASTRO_DEPLOYMENT`, your selection and `default = true` only preselect the entry in that prompt, and the prompt does not change your selection. A run that cannot be asked and names nothing fails. Details are in [deploy.md](deploy.md#3-selecting-the-deployment).

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

`astro link add`, `astro link remove` and `astro link default [--unset]` edit the project's links in `pyproject.toml`. They are described in [Linking from the command line](manifest-reference.md#linking-from-the-command-line).
