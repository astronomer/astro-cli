# The workspace link

A project can link an Astro workspace in its manifest. The workspace's Environment Manager objects (environment variables, Airflow Variables and connections) then reach the project's local Airflow, below every local source, and a name declared `source = "workspace"` is required from it. `astro local start` and Astro Desktop both read the link, and a project starts with the same values whichever one starts it. This page is the contract both implement. How workspace values fit among the local sources is in [secrets.md](secrets.md#precedence).

**Status: experimental.** Astro Desktop sets the link; the CLI reads it and has no command to set it (see [Setting the link](#setting-the-link)).

## The manifest

```toml
[tool.astro]
workspace = "cmws123"      # the linked workspace's id
domain = "astronomer.io"   # the Astro host that workspace lives on
organization = "clorg456"  # the organization that workspace is in
```

- `domain` is the host `astro login` takes, such as `astronomer.io`. Astro
  Desktop writes all three keys together and removes all three on unlink.
- **Absent, `domain` means `ASTRO_DOMAIN` if that is set, else
  `astronomer.io`.** It does not follow whichever login is current, so a
  project reads the same workspace whoever starts it and from whichever app.
  `ASTRO_DOMAIN` is the explicit override: CI sets it beside
  `ASTRO_API_TOKEN`, and Astro Desktop reads it as its own host.
- **`domain` is normalized the way `astro login` stores a login:** lower-cased,
  with any `https://`, `cloud.` and trailing `/` removed, so
  `https://cloud.astronomer.io/` means `astronomer.io`. Written any other
  way it would name no stored login, and the suggested `astro login` would store
  the next one under a different key again.
- `domain` without `workspace` is valid when a deployment link uses the Astro
  login (`method = 'astro'`, the default for Astro links); see
  [Deployment links](#deployment-links). With neither, it is a manifest
  problem: it names a host for nothing.
- `organization` is the id of the organization the workspace is in. It is
  optional, and only valid beside `workspace`: without one it names the
  organization of nothing, and the parser reports
  `organization_without_workspace`.
- **Absent, `organization` means the login's own organization**, the one
  `astro organization switch` last picked for that login. Both apps make this
  choice the same way.
- **A login can read every organization its user belongs to**, without a
  switch. So a project that records its workspace's organization is read there
  whichever organization the login is switched to, the way `domain` keeps it
  on its host.

## Resolving a `source = "workspace"` name at start

In order, identically in both apps:

1. **Local wins.** The project `.env`, the shell and the vaults are asked
   first. A name any of them sets never reaches the
   workspace, so no login and no network is needed for it.
2. **The manifest's domain picks the login.** Both apps read the login stored
   for that domain in `~/.astro/config.yaml` (the per-domain `contexts` entry),
   whatever domain the CLI's current-context pointer names. Switching the CLI
   to another host does not break a project linked to a different one, for
   someone who also has a login there. A stale token for that login is
   refreshed first and saved under that domain; the current-context pointer is
   never moved.
   The read asks under the manifest's `organization`, else that login's
   organization, and never switches the login's organization.
3. **One read per start.** The workspace is read once, with secret values; that
   read supplies Airflow's environment and is the one missing values are
   reported from. Nothing re-fetches to decide what was missing.
4. **A value the read cannot supply is missing, with a cause** from the list
   below.

A value the org's secrets policy withheld is missing, never resolved blank. A
native connection read without secrets counts as withheld, because "no
password" and "password withheld" cannot be told apart.

## Missing values block the start

If any **required** declared value is missing after the steps above, the start
is refused and every missing name is listed with its cause and a fix. An
`optional = true` declaration never blocks.

Both apps offer the same way past it:

- CLI: `astro local start --allow-missing` starts anyway, printing the same list
  as a warning.
- Desktop: the refusal offers **Start anyway**, which does the same.

Starting anyway starts Airflow without those values; it never invents one.

## Causes

The same causes, in the same words, in both apps. `<domain>` is the manifest's
domain, and `<org>` the organization the read asked under: the manifest's
`organization`, else the login's. The words are `pkg/emfetch`'s
(`Cause.TextFor`), so neither app keeps a copy.

| cause | when | message |
|---|---|---|
| not logged in | no login stored for `<domain>` | not logged in to `<domain>`. Log in with `astro login <domain>` |
| session expired | the platform answers 401 | your `<domain>` session expired. Log in again with `astro login <domain>` |
| no access | 403, the manifest names no `organization` | you don't have access to this workspace on `<domain>`. Check your current organization (`astro organization switch`), or ask an org admin |
| not found | 404, the manifest names no `organization` | workspace `<id>` was not found on `<domain>`. Check `workspace` and `domain` in pyproject.toml, and your current organization |
| no access to the organization | 403 or 404, the manifest names `organization` | could not read workspace `<id>` in organization `<org>` on `<domain>`, which pyproject.toml names. Check that you belong to it with `astro organization list`, and `organization` and `workspace` under [tool.astro] |
| secrets withheld | the org disables secret fetching (a 405 or 403 refusal of `showSecrets`) and the value is a secret | organization `<org>` disables Environment Secrets Fetching. Ask an org admin to enable it, or set the value locally |
| no value | the workspace holds the object with no value | the workspace holds no value for it |
| offline | no response | could not reach `<domain>`. Check your connection, or set the value locally |
| no workspace | the manifest sets no `workspace` for a `source = "workspace"` name to read | the manifest sets no `workspace`. Link a workspace to the project in Astro Desktop, or set the value locally |

"No access" and "not found" point at the current organization because, with no
`organization` in the manifest, the read asks under it. With one, a 403 or 404
is that organization's: the login is not a member, or the workspace is not in
it, and switching would not help, so the cause names it and the command that
lists the login's organizations.
"Not found" names the domain because a workspace id asked of the wrong host is
not found, and the fix is usually the login, not the manifest.
"No workspace" is a missing value, not a manifest error, because local wins
with no network needed: a project whose `.env` supplies the name starts without
a `workspace`, and only a name nothing local sets reports the cause.

## What is read, and how

- **Every object, declared or not.** Each environment variable, Airflow
  Variable and connection the workspace holds that nothing local supplies
  reaches Airflow, below the global vault and above a declaration's default. A
  name declared `source = "workspace"` is the one that refuses a start when it
  cannot be read; any other name the read misses simply is not injected.
- **Workspace scope only.** The read is
  `/organizations/{org}/environment-objects?workspaceId=`, the team-shared
  values. Deployment-scoped values are that Deployment's runtime configuration
  and are never read onto a laptop.
- **Keys map one to one.** An object keyed `INCIDENT_CHANNEL`,
  `AIRFLOW_VAR_REGION` or `AIRFLOW_CONN_DB_MAIN` is that env var. A Variable or
  connection stored by its plain name (`region`, `db_main`) is mapped with
  `airflowenv.EnvKeyForStoredVarKey` and `ConnIDForStoredConnKey`. A native
  `CONNECTION` object is re-encoded into `AIRFLOW_CONN_<ID>` through
  `airflowenv.EncodeConnEnv`, the codec Astro Desktop uses, and wins over an
  env-keyed copy of the same connection. A key that cannot be an env-var name
  is skipped with a note.
- **Secret values need the org's permission.** The read asks for secret values
  (`showSecrets=true`). The platform honours that only when the organization
  turns on "Environment Secrets Fetching"; otherwise the read is retried
  without secrets, the non-secret values still resolve, and each secret one is
  missing with the "secrets withheld" cause. Empty values are skipped rather
  than injected blank.
- **One read per run, bounded to 15 seconds,** one list call per object type,
  held in memory. A start that cannot read the workspace goes on without its
  values and prints one line:
  `workspace <id> not read (<reason>): starting without its values. <cause>`,
  unless a required `source = "workspace"` name needs it.
- **Nothing is written to disk.** Values travel to Airflow as `SecretEnv`, so
  Docker mode keeps them out of its compose file too (see
  [secrets.md](secrets.md#what-reaches-airflow)). There is no cache. A
  user whose access is revoked keeps nothing the read fetched; the next start
  gets the cause instead. Only `astro local env <noun> get` prints a value, one
  at a time.
- **Every read is audited** as a read of the Environment Manager API under the
  user's own token, like `astro env variable list --include-secrets`.

Which commands read it:

- `astro local start` and `restart`, in both modes.
- `astro local run` and `astro local shell` with `--with-workspace`. Without
  it, a stopped project's run stays offline and warns which
  `source = "workspace"` names it runs without.
- `astro local env list` and `get`. `list` reads names and metadata only, never
  secret values, and never blocks: a failed read prints
  `workspace <id> not read (<reason>): its values are not listed` on stderr,
  and rows show `workspace (<id>)` or `workspace (unavailable: <reason>)`.
- `astro otto`, which adds the workspace's connections to the analyzing-data
  skill's warehouses at the lowest precedence (a vault connection of the same
  id wins), with an 8-second bound; a failed read is logged and the launch
  goes on.
- Not `astro local check` or `astro package`, which stay offline and say the
  workspace's values were not checked.

Deployment links (`[tool.astro.deployments.*]`) keep their own `workspace`
key. How an `astro` link uses `domain` is below, and how `astro env` and
`astro deployment` use both is under
[Commands that manage Astro](#commands-that-manage-astro).

## Deployment links

A deployment link that proves itself with `method = 'astro'`, the
default for Astro links, uses the same login: `ASTRO_API_TOKEN` if set, else
the login stored for `domain`, refreshed when stale, and never moving the
current-context pointer. The Deployment lookup
goes to that host's control plane under that login's organization. The one
difference is the default: with neither `domain` nor `ASTRO_DOMAIN` set, a
deployment link uses the current login rather than `astronomer.io`, so a
project that names no host keeps working for someone logged in to another host. The
"not logged in" and "session expired" messages are the ones in the table above.

## Commands that manage Astro

`astro env` and `astro deployment` follow the project too. Inside a project,
a command reads its workspace and Deployment in this order:

1. **`--workspace` wins.** Given, it is the workspace. A link name in the
   same command still picks the link's Deployment and host.
2. **A link name is a Deployment.** `--deployment` and the Deployment-id
   argument of `inspect`, `logs`, `update`, `delete`, `hibernate` and
   `wake-up` take an Astro link's name. The command acts on the link's
   Deployment, in the link's workspace, with the login for the link's host
   (the rule in [Deployment links](#deployment-links)). A Deployment id that a
   link holds is read the same way. A link's name wins over a Deployment of
   the same name, and on a command that takes a Deployment name, a value that
   names no link and is not a Deployment id is a Deployment name. On a
   command that takes only an id (the `team`, `user`, `token` and `bundle`
   groups, `astro env`), or as an argument, a value that is neither a link nor
   a Deployment id is refused, and the error lists the project's Astro links.

`--deployment-id`, `--deployment-name` and `--workspace-id`, the spellings
these commands took before `--deployment` and `--workspace`, still work and
mean what they did; help no longer shows them. Given together with the new
spelling, they must name the same thing, or the command stops with a usage
error.
3. **Else the project's workspace.** With no link involved, the workspace is
   `[tool.astro] workspace`, read with the login for the workspace's domain, as
   a `source = "workspace"` value is. `deployment create` and `deployment list`
   use it too; `deployment list --all` does not. In a project that sets no
   `domain`, that host is `astronomer.io` while a link's is the current login's,
   the same split the two sections above describe.
4. **Else the context.** Outside a project, or in one with no `workspace`, the
   command uses the current context, as it always has.

When the login is for another host than the current context's, the command
refreshes that login if it is stale and stops with the "not logged in" message
if there is none. It runs as if `ASTRO_DOMAIN` named that host, and it never
moves the current-context pointer. With `ASTRO_API_TOKEN` or an API key set,
the command stays on the host the environment names: the CLI stores a login
for a token on the host it runs on and makes that host current, so a switch
would move the pointer. A manifest that does not load stops the command, as it
stops `astro af`. When the project picks a workspace or a host
the context would not have, the command says so on stderr:

```
using workspace Example from pyproject.toml
using workspace Example on astronomer.io from pyproject.toml (link test)
```

`delete` asks before it deletes a Deployment named by a link, as it does for an
id, and leaves the link in pyproject.toml.

These commands still act in the login's current organization, which they read
from the context. A workspace the manifest puts in another organization is
named from that organization in the line above, but the command itself runs in
the current one.

## Setting the link

**The workspace link is set from Astro Desktop, and is experimental.** The CLI
reads `workspace`, `domain` and `organization` exactly as this document
describes, so a project linked in Astro Desktop behaves the same in both tools,
but the CLI has no command to set or clear them. Link, switch or unlink the
workspace in Astro Desktop rather than editing these keys by hand.

Astro Desktop writes the three keys together through `scaffold.SetWorkspaceLink`.
An empty organization there means "not given": it keeps the one recorded for
the workspace already linked, and clears it when the workspace changes.
Unlinking removes all three. A switch or an unlink first writes the old
workspace onto each Deployment link that inherited it, so no link moves.
