# Local environment values and the vault

A local Airflow needs values to run: database passwords, API tokens, connection strings, plain settings. This page describes where `astro local` keeps them, the order it resolves them in, how they reach Airflow, and the `astro local env` commands that manage them. The manifest side, declaring what a project needs in `[tool.astro.env]`, is in the [manifest reference](manifest-reference.md#toolastroenv). Values read from a linked Astro workspace are in [workspace-link.md](workspace-link.md).

The CLI and Astro Desktop share all of this: the same stores, the same key grammar, the same precedence, the same link index. A value set in one is read by the other.

## Where values live

| store | holds | encrypted |
| --- | --- | --- |
| the project's `.env` | values set with `--plain` in a project, and anything you write by hand | no |
| the project vault | values set in a project | yes |
| the global vault | values set with `--global`, for any project they are linked to | yes, unless set with `--plain` |

**The vault** keeps one master key in the OS keyring (macOS Keychain, Windows Credential Manager, Linux Secret Service) and stores each value encrypted with AES-256-GCM under `~/.astro/secrets/`, which only your user can read. A value that has been altered or does not belong to the entry it is read for is refused rather than returned. When the keyring cannot be reached, the vault refuses instead of falling back to plain text; `--plain` is the explicit way to store a value without it. A global set with `--plain` is kept in the vault directory unencrypted, so it needs no keyring.

Values are matched by the environment variable they become, so `region` and `REGION` for an Airflow Variable are one entry, as they are to Airflow.

**The `.env` file** is created on the first `--plain` project set, at mode `0600`. Writes merge: an existing key is replaced in place, new keys are appended, and comments, blank lines and hand-typed entries are kept. A connection is written as `AIRFLOW_CONN_<ID>=<json>` and an Airflow Variable as `AIRFLOW_VAR_<KEY>=<value>`, the forms Airflow reads, and `get` and `list` decode them back to their kind. `astro init` puts `.env` in `.gitignore`, and a `--plain` set warns when the file is not covered by `.gitignore`.

## Precedence

One order, everywhere values are resolved (`start`, `restart`, `run`, `shell`, `check`, `list`, `get`, Astro Desktop):

```
project .env  >  shell env  >  project vault  >  global vault  >  linked workspace  >  declaration default
```

- **The project `.env` comes first** because a start applies the whole file over the inherited environment, the way docker-compose reads an `env_file`. For a name both hold, Airflow gets the file's value. This is also what lets a developer shadow a shared secret locally without deleting it.
- **The shell beats both vaults**, so `FOO=bar astro local start` overrides the vault and the workspace, but not a name the `.env` sets. This is the path for CI and for external secret tools.
- **The project vault beats the global vault**: a project value is more specific.
- **The linked workspace** (`[tool.astro] workspace`) is read from Astro's Environment Manager below every local source (see [workspace-link.md](workspace-link.md)).
- **A declaration's `default`** applies only when nothing else supplies the name. A name declared `source = 'workspace'` never falls back to its default.

## What reaches Airflow

Everything that reaches the project is injected, declared or not:

- every entry of the project `.env`;
- the project vault;
- every global vault entry linked to this checkout (see [Which projects a global reaches](#which-projects-a-global-reaches-links));
- every object of the linked workspace that nothing local supplies.

A name more than one source holds goes to the highest in the chain. To keep a global out of a project, narrow its links; a declaration does not filter.

**Declarations are requirements.** A declared name no source supplies refuses `astro local start` (`--allow-missing` starts without it). Types, enums and `conn_type` are validated and reported as warnings. `astro local env list` shows each undeclared value the project gets, and `astro local check` and `astro package` name the local ones, since they will not follow the project to a Deployment or a teammate's clone.

**Vault and workspace values stay off disk.** In Docker mode they are passed to the containers through the environment of the compose process, never written into the compose file the CLI generates and never put on a command line. Plain `.env` values are written into that file, as docker-compose would read them anyway. As with any container, `docker inspect` can show a running container's environment.

## Which projects a global reaches: links

A global vault entry can reach every project, no project, or a list of projects. Astro Desktop calls these *auto-linked*, *not linked* and *linked*, and the two tools share the setting, so a global linked in one is linked in the other.

- **New globals start not linked.** `set --global` of a new name reaches no project until you link it, and says so. `set --global --auto-link` creates it reaching every project. Updating an existing global keeps its links.
- **A link names a project.** Linking a project also reaches its git worktrees, wherever they live; `--this-checkout` links one worktree only.
- **Project values never consult links.** They already belong to one checkout.
- **It fails safe.** Reaching too few projects is the safe failure; reaching too many would put a credential into another project's Airflow. If the link settings (`~/.astro/secrets/links.idx`) cannot be read, no global reaches any project until they are fixed, the missing-value report names the file, and `link` and `unlink` refuse to change it. Project values keep working.
- Deleting a global also deletes its links, so a new global of that name starts not linked.

## `astro local env`

The command group lives under `astro local` because `astro env` is the cloud Environment Manager. It mirrors that sibling's nouns, aliases, verbs and connection flags, so a token names the same object in both trees.

```
astro local env <noun> set       <name>   [--project | --global] [--stdin | --value <v>] [--plain] [--replace-secret] [--auto-link]
astro local env <noun> get       <name>   [--project | --global] [--plain]
astro local env <noun> list               [--project | --global | --all]
astro local env <noun> delete    <name>   [--project | --global] [--plain] [--undeclare]
astro local env <noun> declare   <name>   [annotation flags]
astro local env <noun> undeclare <name>
astro local env <noun> link      <name>   [DIR...] [--this-checkout | --auto-link]
astro local env <noun> unlink    <name>   [DIR...]
astro local env list                      [--project | --global | --all]
```

Every command takes `--output json`. The nouns and their aliases:

| noun | aliases | kind |
| --- | --- | --- |
| `variable` | `var`, `variables`, `vars` | a plain environment variable |
| `connection` | `conn`, `connections` | an Airflow connection |
| `airflow-variable` | `airflow-var`, `airflow-vars`, `airflow-variables` | an Airflow Variable |

`list` has alias `ls` and `delete` alias `rm`. There is no default kind, so no name collides with a subcommand: `astro local env variable set conn` sets an env var called `conn`.

**Scope.** Inside a project the default scope is the project, elsewhere global. `--project` and `--global` force one. On `get`, no scope flag resolves the whole chain and reports the winning source; a scope flag reads only that scope's stores.

### `set`

**Where it stores the value:**

- No flag: the vault, encrypted, at the chosen scope.
- `--plain`: unencrypted. A project value goes to the `.env`; a global goes to the vault, marked plain. This is the path where there is no keyring.
- `--secret` is deprecated and does nothing; `--secret=false` means `--plain`. `--plain` with `--secret` is refused.

**Refusals:**

- **No keyring.** A default set refuses rather than fall back to a file, naming `--plain`. Silently writing plaintext would make the routing a lie.
- **Downgrading a secret.** `set --plain` of a name the scope's vault holds encrypted is refused: `delete it first, or pass --replace-secret to store it as plain text`. `--replace-secret` is refused without `--plain`.
- **A declared secret.** `set --plain` of a name the manifest declares `secret = true` is refused in either scope. A declared connection is always secret. A `--plain` set reads the manifest to know this, so a manifest that does not parse refuses it rather than guess. A set with no flag does not read the manifest.

**One home per scope.** A project set removes the name from the project's other store: a vault write deletes the `.env` copy, and a `--plain` write deletes the vault copy, since a stale `.env` copy would keep winning and keep the credential on disk. A `--global` set never touches project values; when the project holds the same name, it warns that start uses the project copy and names the `delete --project` command.

**Input.** A value is never a bare argument, which would land in shell history and `ps`. With `--stdin`, or when stdin is not a terminal, it is read from stdin; otherwise a no-echo prompt asks for it. `--value <v>` is the scripting escape hatch and is documented as leaking into history. A `NAME=value` form is not accepted. This applies to every set, secret or not.

**Connections.** `connection set <id>` takes a whole connection, as a URI (`postgres://user:pass@host:5432/db`) or the connection JSON, or field by field with the cloud sibling's flags: `--type`, `--host`, `--login`, `--password`, `--schema`, `--port`, `--extra` (no short forms, since `-p` beside `--project` would read as the scope). Both shapes are encoded through the shared `airflowenv` codec into identical records.

### `get`

Prints the value on stdout, the one deliberate reveal. With no scope flag it resolves the whole chain. `--project` reads the `.env` and the project vault, returning the one the chain uses and noting the other; `--global` reads the global vault; `--plain` reads only the `.env`. For a global, `Reach: auto-linked (every project)`, `Reach: /a, /b (missing)` or `Reach: not linked (no project)` goes to stderr, and `--output json` adds a `reach` object.

### `list`

Shows every declared name with the source it resolves from, plus `required`, `secret` and `description`; values are never printed. The sources are:

| source | meaning |
| --- | --- |
| `project` | the project `.env` |
| `shell` | an exported variable |
| `vault` | the project vault |
| `vault (global)` | the global vault |
| `workspace (<id>)` | the linked workspace |
| `workspace (unavailable: <reason>)` | the workspace could not be read |
| `default` | the declaration's default |
| `absent` | nothing supplies it |

A declared name nothing supplies is noted with the `set` and `undeclare` commands to run (`set_hint`, `undeclare_hint` in JSON). A value present in a store that no declaration names is listed too, as an orphan of its tier (`"orphan": true`), with the command that removes it (`remove_hint`). When it reaches this project's Airflow it is noted `not declared (declare it to make it a requirement: …)`, with `"applied": true` and a `declare_hint`, since it is injected already and a declaration would only make the project require it. A global that reaches the project but is shadowed by a project copy carries no mark.

`--project` and `--global` narrow the list to that scope's stores. `--all` also lists every project `.env` the CLI can find, and globals that do not reach this project, marked `not linked here` with the `link` command (`"not_linked_here": true`, `link_hint`), or with the index problem (`links_down`).

`list` is built from the schema and source metadata, never from decoded values, so its JSON is value-free and safe for agents. With `--output json` it prints one object with the rows under `entries` (`{"entries": [...]}`, `[]` when there are none); `variable list` and `connection list` print the same shape, narrowed to their kind.

### `delete`

Removes a value, never a declaration. In a project it removes the name from both project stores; `--plain` removes only the `.env` copy; `--global` removes the global. When the project declares the name, it says what is left, without printing a value: another source supplies it (naming the source or the default), nothing does, or the next start will refuse, with the `set` and `undeclare` commands. In JSON that is `remainder` (`supplied`, `absent` or `required`) with `source`, `set_hint`, `undeclare_hint`, `workspace` and `manifest`. It reads only local sources; with a workspace linked and nothing local supplying the name, it adds that the workspace may still supply it. The classification is `envschema.RemainderAfterDelete`, which Astro Desktop calls too.

`--undeclare` also removes the declaration from the current project's `pyproject.toml` (with `--global` too) and reports `"undeclared": true`. It is refused before anything is deleted when the manifest does not load, does not declare the name, or there is no project.

### `declare` and `undeclare`

Edit the project's `[tool.astro.env]`, so they refuse `--global` and take no `--plain`. They write through the same code Astro Desktop uses, which refuses a result that would not load and leaves the file as it was. `declare` changes only the annotations whose flags are passed: `--description`, `--optional` and `--source workspace|local` on every noun; `--type`, `--enum`, `--secret`, `--default` and `--no-default` on the variable nouns; `--type` (the `conn_type`) on `connection`. An env-form key is declared by its plain name (`AIRFLOW_VAR_REGION` declares `region`). The flag table is in the [manifest reference](manifest-reference.md#declaring-from-the-command-line).

### `link` and `unlink`

Decide which projects a global vault entry reaches.

```sh
astro local env connection link warehouse                       # this project and its worktrees
astro local env connection link warehouse ~/src/etl ~/src/reports
astro local env connection link warehouse --this-checkout       # this worktree only
astro local env connection link warehouse --auto-link           # every project (removes the row)
astro local env connection unlink warehouse                     # stop reaching this project
astro local env connection unlink warehouse /Users/me/old-etl   # drop a project that moved
```

- Each `DIR` resolves to its enclosing project, then to its project home, so a link reaches the project and all its worktrees. `--this-checkout` links the checkout's own path. Linking an auto-linked entry narrows it, and says `now reaches only …`.
- `unlink` removes both spellings of each directory. A path that no longer exists is matched as written. Unlinking the last project warns that the entry now reaches nothing; unlinking an auto-linked entry is an error.
- Only global vault entries can be linked; a project value or a name the vault does not hold as a global is refused.

### JSON shapes

```
# list (never includes values)
[{"kind":"env","name":"API_URL","required":true,"secret":false,"source":"project"}]

# get
{"kind":"env","name":"API_URL","source":"project","value":"https://..."}
{"kind":"conn","name":"warehouse","source":"vault (global)","value":"...",
 "reach":{"auto_link":false,"projects":[{"path":"/Users/me/etl","exists":true}]}}

# set
{"kind":"env","name":"API_URL","scope":"project","status":"set"}

# delete
{"kind":"env","name":"API_URL","scope":"project","status":"deleted","remainder":"required",
 "set_hint":"astro local env variable set API_URL",
 "undeclare_hint":"astro local env variable undeclare API_URL","manifest":"/path/to/pyproject.toml"}

# declare / undeclare (status: declared, undeclared or unchanged)
{"kind":"env","name":"API_URL","status":"declared","manifest":"/path/to/pyproject.toml"}

# link / unlink (status: linked or unlinked)
{"kind":"conn","name":"warehouse","status":"linked",
 "reach":{"auto_link":false,"projects":[{"path":"/Users/me/etl","exists":true}]}}
```

An unusable index shows in `reach.error`.

### The missing-value report

`astro local start` lists every missing declared value at once, each with the command that sets it:

```
connection "warehouse" (postgres) is required and not set.
  provide it:  astro local env connection set warehouse --project
```

A declared global linked elsewhere is reported with where it is linked, each path that no longer exists marked `(missing)`, and the `link` command.

## Astro logins

The access and refresh tokens `astro login` saves live in the vault too, one entry per context per config file, so `~/.astro/config.yaml` keeps only the non-secret half of a login (domain, email, expiry, organization). `secrets.Logins` (`pkg/secrets/login.go`) holds the rules, and the CLI (`config/login.go`) and Astro Desktop both apply them, so the two read each other's logins.

- **What the config says.** A login in the vault leaves `token: "Bearer "` and an empty `refreshtoken`, which every earlier CLI and desktop build already reads as signed out. Both fields empty is signed out, and outranks the vault. Anything else is a login in the config as written, the newest one there is: the next read moves it into the vault. A login leaves the config only after the vault has accepted it.
- **Older builds.** An older CLI or desktop sees a signed-out context and asks for a login; that login goes to the config, and the next current build moves it into the vault. An older build that signs out empties the fields, and the current build treats the context as signed out. Switching between old and new builds costs at most a login.
- **No keyring.** Headless Linux, SSH sessions, containers and CI keep the login in the config exactly as before; nothing refuses, including sign-out. A failed keyring attempt is not repeated in the same process, other processes wait an hour before moving a login again (a save still tries, since one session's failure says nothing about another's), and reading the master key times out, so an unattended keyring cannot stall a command indefinitely. Once a keyring answers, the next read or save moves the login.
- **A login that cannot be read** (keyring unavailable in this session, a damaged entry) reads as signed out with a warning, and is not deleted by code that writes back the empty fields it read; only signing out removes it.
- **Another process's login.** Writing the config for any other reason keeps the token fields another process saved since this one started, unless this process changed that context's login itself.
- **A lost master key.** Logins do not keep a vault whose master key is gone from making a new one, since logging in again recovers them: the old logins are removed and read as signed out. Other encrypted values still refuse a new key.
- **Environment credentials.** `ASTRO_API_TOKEN` and the other variables are read before any saved login, as before.
- **Logout** removes the vault entry and leaves both fields empty.

## Security notes

- **Encrypted by default.** Only a value set with `--plain` is unencrypted: in the project's `.env`, or a global set with `--plain`.
- **Plain values are protected by file permissions** (`.env` and vault files `0600`, the vault directory `0700`), the `.gitignore` guardrail, and full-disk encryption (FileVault, BitLocker, LUKS). An unlocked session can read them, as it can any dotfile, and backup or sync tools (iCloud, Dropbox, Time Machine) copy them in the clear.
- **The keyring's limits are explicit.** A vault copied to another machine cannot be decrypted there; a headless machine with no keyring cannot use the encrypted vault, and a default set says so. CI should use the shell environment, which needs no keyring and outranks everything but the `.env`.
- **The vault is local storage, not team sharing.** A machine-bound key cannot be shared. The team story is the workspace's Environment Manager values (see [workspace-link.md](workspace-link.md)), where rotation and access are handled server-side.

## Using an external secrets tool

Because the shell outranks both vaults and the workspace, any tool that exports values into the environment works with no astro change. Keep those names out of the project `.env`, which a start applies over the shell.

```sh
sops exec-env dev.enc.env -- astro local start
op run --env-file=.env.tpl -- astro local start
direnv   # .envrc exports the values; astro local start inherits them
```

For a sops-encrypted env file committed to the repo, `dev.enc.env` is the conventional name. astro does not know such values came from a tool, so a missing-value hint cannot name the tool.

## CI

A pipeline sets environment variables the way it already does. They outrank everything but the `.env`, need no keyring, and `astro local start` reads them.
