# Architecture

Astro CLI 2.0 is the second major version of the Astro CLI. Its centerpiece is `astro local`, a local-Airflow command family that replaces the `astro dev` tree, built on a `pyproject.toml` manifest.

This page holds the rules the code agrees on: the layers, the sub-module boundaries, the output contract, and where local state lives. The other docs:

- [manifest-reference.md](manifest-reference.md): every key in `pyproject.toml`.
- [secrets.md](secrets.md): local environment values, the vault, and how they reach Airflow.
- [workspace-link.md](workspace-link.md): reading values from a linked Astro workspace.
- [instances.md](instances.md): which Airflow a command talks to.
- [deploy.md](deploy.md): `astro deploy` and `astro package`.
- [install.md](install.md): installing the CLI and converting a project made by Astro CLI 1.x (a 1.x project).

## One core, three consumers

The CLI is a library that ships a binary. Three consumers share the core:

1. the `astro` binary (this repo);
2. Astro Desktop, a desktop app for local Airflow development, which imports the `pkg/*` sub-modules as Go modules;
3. editor integrations, which shell out to the CLI and read `--output json` (see [Output](#output)).

Anything a second consumer needs lives in `pkg/` as a sub-module. `internal/` holds CLI-private glue: plan building, resolution order, cobra wiring. Some functions in `pkg/*` are called only by Astro Desktop, which is why `make deadcode` skips the sub-modules.

## Layers

```
cmd/        parses flags, calls one function, formats output. Nothing else.
internal/   CLI-private logic. May import pkg/. Never imports cmd/.
  platform/ one directory per control plane (astro, apc), each holding that
            platform's domain packages and the transport they talk through.
pkg/        shared sub-modules, plus ordinary root-module packages.
```

Only the `pkg/` directories with their own `go.mod` are sub-modules. The rest are ordinary packages in the root module.

### Core and shell

The rules divide the code in two, after "functional core, imperative shell". The **core** returns data and typed errors and leaves the I/O to its caller: `cmd/local`, `cmd/cliout`, and the packages below `cmd/` that archlint lists as core. The **shell** does the I/O itself: it prints, prompts and reads `config/`. It is mostly code inherited from Astro CLI 1.x, such as the rest of `cmd/` and both control-plane platforms under `internal/platform/`. Shell code may import the core; the core never imports the shell's `cmd` tree.

Logic that is not about one platform does not import `internal/platform/`. It takes an interface and `cmd/` wires the implementation, so `internal/deploy` reads the same whichever control plane it ships to. The seams that may reach a platform are `internal/emenv` and `internal/instancelocate`. The core packages do not read `config/` either, except the four config readers `internal/astrosession`, `internal/containercfg`, `internal/emenv` and `internal/instancelocate`.

Below `cmd/`, the core never prints and never exits. Libraries report progress through callbacks (`localrt.Callbacks`) and return typed errors; each frontend decides how to render. The CLI writes text, desktop draws UI.

[`internal/archlint`](../internal/archlint/archlint_test.go) enforces these rules as tests:

| test | rule |
| --- | --- |
| `TestInternalNeverImportsCmd` | nothing under `internal/` imports `cmd/` |
| `TestCorePackagesNeverImportConfigOrShellCmd` | `cmd/local`, `cmd/cliout` and the core packages stay off `config/` and the shell `cmd` tree |
| `TestOnlyTheSeamsReachIntoAPlatform` | only the seams import `internal/platform/` |
| `TestEveryInternalPackageIsAccountedFor` | every `internal/` package is listed as core, a core config reader, or shell |
| `TestCorePackagesBelowCmdNeverPrintOrExit` | no `fmt.Print*`, `os.Exit` or `log.Fatal*`/`Panic*` in core packages below `cmd/` |
| `TestEveryPkgSubmoduleIsLinted` | every `pkg/*/go.mod` is in the Makefile's `LINT_SUBMODULES` |
| `TestEveryLintedSubmoduleIsHeldToTheNoPrintRule` | every linted sub-module is under the no-print rule |
| `TestTheAuthDoorsStayOptional` | `pkg/instances` and the instance locators link neither auth door nor a cloud SDK |
| `airflowkey_test.go` | `[tool.astro]` carries no Airflow-version field, and nothing reads one |

Contract structs (`Plan`, `Deps`, options) pass by value, so the `hugeParam` lint is suppressed for the packages that carry them (the path list is in `.golangci.yml`).

## Sub-module rules

A `pkg/` sub-module:

- has its own `go.mod` (`module github.com/astronomer/astro-cli/pkg/<name>`);
- imports neither the parent module nor a sibling sub-module, except the declared dependencies below; if it needs a path or a config value, it takes it as an argument;
- keeps its dependency list near-empty, since every dependency is one desktop inherits;
- is wired into the root module with a require + replace pair, added with its first in-repo consumer, and listed in `LINT_SUBMODULES`.

If a candidate cannot be made into a self-contained sub-module in an afternoon, it belongs in `internal/` instead.

### Declared sibling dependencies

A sub-module may import a sibling only when a second implementation of what the sibling owns would be a second answer to the same question. Each one is listed here; a new one is added here, not assumed. archlint does not check this list, so review does.

| module | may import | why |
| --- | --- | --- |
| `airflowenv` | `connmodel` | the codec exists to encode the connection type |
| `envschema` | `airflowenv` | declared names are validated with `ValidEnvKey`, `ValidVarKey` and `ValidConnID`, and encoded as `AIRFLOW_VAR_`/`AIRFLOW_CONN_` keys by the same rules |
| `connwarehouse` | `connmodel`, `fsatomic` | it maps a connection into an entry of `~/.astro/agents/warehouse.yml`, written atomically |
| `secrets` | `fsatomic` | atomic value writes, the link index, and its lock |
| `proxy` | `fsatomic` | atomic writes of `routes.json` and the daemon record |
| `localrt` | `airflowrt`, `container`, `fsatomic`, `manifest`, `proxy`, `uv` | it orchestrates the runtime primitives (PID files, env, health), routes, the container engine, venvs and atomic state. It reads two manifest fields: `[tool.uv] environments`, to stop before uv on a platform the project leaves out, and `[tool.uv] constraint-dependencies`, passed to `uv pip install` on a hot install, because an embedder that sets `UVOptions.NoConfig` makes `uv pip` skip them |
| `imagebuild` | `localrt/rt`, `manifest`, `runtimeversions`, `airflowrt` | it reports progress in the runtime's vocabulary (`Callbacks`, `LogLine`), through the dependency-free `rt` leaf rather than `localrt`; drops the Airflow requirements with `manifest.WithoutAirflow`; names an Airflow 2 base from the runtime catalog; and reads build-secret mounts with `airflowrt.SecretMounts` |
| `checks` | `manifest`, `platformversions` | `manifest.WithoutAirflow`, and the table of Airflow versions MWAA and Composer offer |
| `scaffold` | `manifest` (with `tomledit`), `envschema`, `airflowenv`, `connmodel`, `fsatomic`, `secrets`, `runtimeversions`, `airflowrt` | it writes the manifest (`EditManifest`, the one writer of an existing `pyproject.toml`, atomically) and must agree with its parser; it writes `[tool.astro.env]` declarations with the parser's own grammar (`envschema.DeclarationName`, `DeclarationTable`); converting a 1.x project carries its `airflow_settings.yaml` values into the vault through a caller-scoped `SecretWriter` speaking `secrets.Kind` and `secrets.Key` (it never opens a store, so the keyring stays out); it reads the runtime catalog's fallback series, `requires-python` and Astro build without fetching; and it reads a Dockerfile's `FROM` exactly as Docker mode does (`airflowrt.ReadDeclaredBase`). `uv` is test-only |
| `instances` | `manifest`, `airflowapi`, `airflowrt` | a link is manifest data; it produces `airflowapi`'s `Transport` and `CredentialSource`; and `airflowrt` owns the local Airflow's provisioned account |
| `instancelocate` | `instances`, `googleauth` | it resolves an `Instance`, and reading a Composer address needs Google credentials. Only the Composer half lives here; the Astro half needs the login context and lives in `internal/instancelocate` |
| `awsauth`, `googleauth` | `instances`, `airflowapi` (and, test-only, `manifest`) | each is an auth door implementing an `instances.Provider` |

The auth doors are separate modules so that requiring `pkg/instances` costs neither the AWS nor the Google SDK. The dependency runs one way: the core never imports a door (`TestTheAuthDoorsStayOptional`).

A dependency's own `replace` lines are never honoured, so a consumer of a module with sibling dependencies needs a require + replace pair (and a `go.work` `use`) for each sibling too.

`pkg/scaffold` also builds the prompt that hands an Airflow upgrade to Otto (`AirflowUpgradePrompt`), so `astro local upgrade airflow --with-otto` and Astro Desktop send Otto one text.

### Formats with readers outside this repo

- **`pkg/proxy`'s `routes.json`.** astro 1.x CLIs in the field write and read it and never auto-update. Add fields freely; a reshape means a new file. The v2 state record is the source of truth, and `routes.json` is a compatibility view.
- **Keyring service names and on-disk secret formats.** After the first public release, users' existing data constrains them: a change needs a migration that keeps the old name or format readable for at least one release.

### Code conventions

- Tests match the package they are in. testify and plain `testing` both exist; don't rewrite either style, and don't mix them within a package.
- Windows handling uses build tags only when code won't compile cross-OS, and a `runtime.GOOS` switch when picking a value.

## Output

The output contract covers the whole CLI, and lives in [`cmd/cliout`](../cmd/cliout): the `--output` flag, `Renderer.Emit` and its stream twin `Renderer.EmitEvent` (the single door every payload leaves by, so a test can watch what a command publishes), and `cliout.Execute`, which runs the root and reports every failure. Formats are `text` (the default) and `json`; a command offers another only for a special use.

`cliout` is the one place `-o/--output` is registered and parsed: `cliout.AddOutputFlag(cmd, &v)` registers it, with the help `Output format: text or json`, and binds it to `v`, a `cliout.Format`. The flag validates its own value: an unknown one is refused while cobra parses flags, before any pre-run refreshes a token or records telemetry, with `unknown output format "x" (supported: text, json)` as a usage error. So `v` only ever holds a format the command offers, and the command reads it as it is: `cliout.Renderer{Format: v, Out: out}`, with nothing to parse and no error to handle. A command with more to say about a refused value gives it to `cliout.OnBadFormat`, after `AddOutputFlag` on the same command (`astro env list` explains why it has no dotenv). A payload that costs something to build and that the text does not read is handed to `Emit` as a `cliout.Lazy`, which is built only in json mode. A command that offers a format beyond those two declares it as an extra, once, where it registers the flag: `astro env variable list`, `get` and `export` take `dotenv` (`AddOutputFlag(cmd, &v, formatDotenv)`), and `astro deployment inspect` takes `yaml`. The extra shows in the help (`text, json or dotenv`) and in the error's list, and nowhere else. A renderer below `cmd/` may not import `cmd/`, so it takes the command's `cliout.Renderer` as an `output.Emitter` (`Emit(v, text)`): the `pkg/output` lists and `internal/platform/astro/env`'s writers draw only the text and hand the value to it, so their json is the same encoder's. Those packages render; they neither parse nor encode json. `astro deployment inspect` hands its Renderer to `inspect.Print` the same way, with the YAML as its text, so `-o yaml` reaches the Renderer as text. The one format beyond the two they have is `dotenv`, which the command writes itself with `env.WriteVarDotenv`. So a Renderer only ever sees text or json, and `Emit` panics on anything else: a Format that did not come through the flag (`""`, or a value no command offers) is a programming error, not a quiet text rendering. One exception remains: `astro deploy`'s `--output`, which has no `-o`, is a plain string flag because outside a pyproject.toml project it is ignored, and is checked with `cliout.ParseFormat` in `cmd/astro/deploy.go`, only inside one.

Every command in the core tree (`cmd/local`) supports `--output json`: `astro local *`, `astro init`, the root aliases `astro start`/`stop`/`logs`, `astro af`, `astro use`, `astro link` and `astro package`. `TestTreeInvariants` in `cmd/local` fails on a runnable command that lacks it. The rest of the CLI is converging on the same rule: `TestEveryCommandCanReachOutputJSON` in `cmd` fails on a visible, runnable command in any of the trees the tree-wide tests build (see Help, below) that cannot reach an `--output` offering json, except the ones its `lacksOutputFlag` list names for that tree's platform, and that list may only shrink. `astro login`, `logout`, `auth login`, `auth logout` and `otto` are exempt by design, as are the requests the `api` family makes, which print the API's own response and shape it with `--jq` and `--template` (its `ls` and `describe` publish the CLI's own listing and schemas, and take `-o` like every other command). A list is one object with the list under a named key, `{"dags":[...]}`, and `[]` when empty, so a script reads it with one parse and fields such as a total can be added later. Only streaming surfaces (logs, events, build output, `astro local check`'s findings) emit NDJSON, one object per line; a list is not a stream. A result (and the error object) is pretty-printed and colored when stdout is a terminal and compact on one line when it is piped or redirected, while a stream's records go through `EmitEvent` and are one line always, on a terminal too. `astro af` lists use the key names of the standalone `af` CLI they stand in for (`dags`, `dag_runs`, ...), so its skills work unchanged. Human output is the default rendering of the same data, not a separate code path.

### Adding `--output` to a command

The commands in `lacksOutputFlag` gain `-o json` a family at a time (`deployment variable`, then `deployment token`, and so on), some before the 2.0 release and some after. A conversion must not break anyone, so each one follows three rules.

1. **Text mode does not change.** The same stdout, the same wording, the same exit codes (with the one exception rule 3 describes). JSON is the new path. Under `-o json`, stdout holds the payload and nothing else: progress, warnings and notes go to stderr (`env_link.go`'s `deploymentPickupNote` is one).
2. **A new JSON shape is free; changing it later is breaking.** So design it once. Keys are snake_case. A list is an array under a named key (`{"tokens":[...]}`), `[]` when empty, never `null`, never a bare top-level array, never NDJSON. A create or update returns the object as it now is, not a confirmation sentence. The shape is pinned once, by a schema golden: add each type the command publishes to its tree's `publishedPayloads` (`cmd/local/schema_cases_test.go`, `cmd/astro/schema_test.go`, `cmd/apc/schema_test.go` for the APC (Houston) platform, `cmd/schema_test.go` for the commands the root builds itself: `version`, `context`, `config`, `telemetry`, `auth token`, and `cmd/api/schema_test.go` for `astro api … describe -o json`), run `make update-schemas`, and read the file it writes. A type that reaches `Emit` with no golden fails the package's run, as does a key in a golden that is not snake_case: every tree arms the same watch (`cliouttest.Watch`) in its `TestMain` and runs the same key check (`cliouttest.KeyProblems`), and a tree that pins shapes keeps a `minWatchedPayloads` floor above 0. The command's own tests then decode what it printed and assert what it means: the exit code, what happened to each input, what a list holds, `[]` when it is empty. They do not restate the shape, and in text mode they check that the messages and rows are there, not how a table pads them. Text is pinned byte for byte only where something parses it, as deploy-action parses `astro deployment inspect`.
3. **A command that exits 0 after failing is a bug.** Fixing it is a bug fix, not a breaking change. It still goes under the PR's `## Breaking changes`, so the release notes say that a script which used to see 0 will now see 1.

The work itself:

- Register the flag with `cliout.AddOutputFlag`, bound to a `cliout.Format` the command reads as it is, and publish through `cliout.Renderer.Emit`, with the text renderer printing exactly what the command printed before.
- Write the text renderer with [`cmd/cliout/text.go`](../cmd/cliout/text.go), which is the standard library and nothing else. `cliout.Text` (or `cliout.WriteText` outside `Emit`) hands the renderer a `bufio.Writer`: its error is sticky, so the renderer writes its lines unchecked and the one error comes back at the flush, instead of every `Fprintln` wrapped in `if _, err := ...`. A list is a `cliout.Table` (a header, rows, and the message for no rows), which lays its columns out with `text/tabwriter` the way the CLI's tables always looked: `r.Emit(v, cliout.Text(table.Render))`. It is `pkg/texttable.Table` under another name, so code below `cmd/` draws the same table. `pkg/printutil.Table` is the old way; its uses move over a family at a time. `deployment variable`, `deployment token` and `astro af config` are the examples to copy.
- Move the command toward the core. The platform function returns a result (what happened to each input, the object afterwards) instead of printing it, and `cmd/` renders it in either format. `astro deployment variable create` does this, and is the one to copy: `VariableModify` returns one outcome per input plus the variables afterwards, and `cmd/astro/deployment_variable_render.go` is the only place deciding how any of it looks.
- Delete the command's entries from `lacksOutputFlag`. `TestOutputFlagAllowlistOnlyShrinks` fails on an entry left behind for a command that now has the flag. Nothing is ever added to the list.
- If the command asks anything, check that each prompt names the flag that answers it (`input.AnsweredBy("--yes")`). Under `-o json` the prompt is refused as `input_required`, and without the flag named the message can only say "pass the answer as a flag". See [Prompts](#prompts).

Failures need no work of their own: once the command has `--output`, `cliout.Execute` reports them as below.

The property as a whole is checked end to end, against the built binary: `TestEveryCommandWritesOnlyJSONToStdout` in `e2e/` walks every command the binary has (through `astro __complete` and each command's `--help`), runs each with `--output json` logged out, logged in to an Astro or an APC API that refuses connections, and inside a project, and fails on anything on stdout that is not the one result, the stream's lines, or the one error object, on an error on stderr as well, and on a group run bare that is not a usage error. A new command is run with no change to the test; one it must not run, or that has no `--output json` by design, is listed there with the reason.

### Failures

Under `--output json`, a command that fails writes one object on stdout and nothing on stderr: `{"error": ..., "code": 1, "kind": ...}`. This holds for every command whose `--output` is `json`, whichever tree it is in, and for every way it can fail: in its run, in a pre-run (no login), or before either, on a flag or argument cobra rejects. `code` is the exit status the process ends with. `error` is prose for a person and may be reworded. `kind` is the stable name for which failure it is, and is contract: a consumer branches on it instead of matching a sentence. It is snake_case, the same vocabulary as a check finding's `kind` (`import_error`, `duplicate_dag_id`). A command with no `--output`, or in text mode, prints `Error: ...` on stderr, and its usage unless it silenced that, as cobra always has.

Some failures publish something else instead, and a consumer has to expect them (neither adds a second object):

- a command that already wrote a richer structured payload (`cliout.JSONShown`): a start blocked on missing env values publishes that list;
- a command that carries its own exit code (`cliout.ExitError`): `astro local check` publishes its NDJSON summary, `astro af runs trigger-wait` its result, and `astro local run` exits with the child's code.

### Commands in messages

The CLI writes a command, flag, path or key in its own output as plain text, with no backticks: "astro dev start was removed in Astro CLI v2. Use astro local start instead." That holds for errors, warnings, notes, prompts, next steps and help. A terminal prints a backtick as it is, and a script or Astro Desktop reading an error object's `error` or a payload's notes gets it as it is, so markdown in a message is noise wherever it lands. Where a bare command reads ambiguously mid-sentence, the sentence is rephrased ("The set command creates ...", "then use astro local start --docker") rather than quoting it. These docs are markdown and keep their backticks.

Two tests hold the rule. `TestMessagesWriteCommandsAsPlainText` in [`internal/archlint`](../internal/archlint/backticks_test.go) parses every non-test Go file in the repo, sub-modules included, and fails on a string literal holding a backtick. Its `backtickAllowed` list names what may hold one because it is markdown or code where the backtick means something: the README and AGENTS.md `astro init` writes, the upgrade prompt handed to Otto, and one shell-metacharacter set. An entry no longer needed fails the test too. `TestHelpHasNoBackticks` in `cmd` renders every help page in every tree, hidden commands included, and fails on a backtick, which also catches text assembled at run time. Text the CLI passes through from uv, Airflow or an API is printed as it came.

### Prompts

A command running with `--output json` never prompts. An agent or a script reading json has nobody at the keyboard, so a question would hang it. A question it would have asked fails with kind `input_required` instead. The failure names what was asked and, where the command knows it, the flag that supplies the answer (`--yes`, `--name`). A command with no such flag says to pass the answer as a flag. Under `--output json` a prompt is refused before it writes or reads anything, whether or not stdin is a terminal.

In text mode a question is asked on stderr: the `pkg/input` prompts, the pickers that are not handed a writer of the command's, the warning a confirmation shows, and the whole login flow (the welcome, the Enter prompt or the link, the progress). stdout carries a command's results only, so with stdout redirected to a file the person still sees what they are asked, and the file holds no prompts. The root's pre-run writes its notes and its logs to stderr too.

There is one choke point. `cliout.Execute` installs a process-level guard for the run (`pkg/input.SetGuard`), and the guard reads the command's own parsed `--output` when a question comes up. Every prompt consults it:

- the `pkg/input` primitives `Text`, `Confirm` and `Password`, which return a `*input.RequiredError` and read nothing;
- any prompt that reads its own reader (`astro local`'s `[y/N]`) calls `input.MayAsk` first. `pkg/picker` does so for every numbered table a person picks a row of: it is the one picker, so a new one is a `picker.List` with its own `About`, `AnsweredBy` and invalid-selection error, not a table and an `input.Text` of its own.

A call site passes `input.AnsweredBy("--yes")` to name its flag, and `input.About("a workspace")` when its prompt text is a bare `> `. The guard is process-level rather than a context value because the shell platform packages ask from call chains that carry no context.

A command that already refuses to ask when it has no terminal marks that refusal with `input.Required(err)`, so it keeps its own words and gains the kind. `astro deploy` with no target is one. Others are `astro local`'s confirmations without `--yes`, a link picker with no NAME, and a deployment several links make ambiguous. In text mode with no terminal, nothing else changes: other prompts still read stdin as before.

### Exit codes

| status | meaning |
| --- | --- |
| 0 | success |
| 1 | the command failed, including `input_required`: a question it could not ask. Not 2, because a question can come after work has started (a 1.x project's DAG deploy asks about an empty `dags/` after it has created the deploy record), so the "nothing ran" that 2 promises would not hold. The same refusal without a terminal has always exited 1. A script that needs to tell it from other failures reads `kind` |
| 2 | usage error: an unknown flag or subcommand, a flag value the command does not accept (`--output yaml` included), the wrong number of arguments, a missing required flag, a group run with no subcommand under `--output json`, whether it has a RunE of its own (`cliout.GroupHelp`) or not (`cliout.Execute` answers it); in text mode it prints its help and exits 0, as before. Nothing ran |
| 130 | interrupted (Ctrl-C or SIGTERM); the command unwound what it had started |
| the command's own | `astro local check`: 1 when a check failed, 2 when it could not reach a verdict (no environment, no project). `astro af runs trigger-wait` (and `astro local af runs trigger-wait`): 1 when the run failed, 2 when the wait timed out. `astro local run`: the child's status |

The command-specific 2s predate the usage 2. A script that runs one of those commands tells them apart by what it published: a usage error under `--output json` is an error object with kind `usage`, while `check` and `trigger-wait` publish their own result.

### Kinds

`cmd/local` names the failures of its commands ([`cmd/local/problemkind.go`](../cmd/local/problemkind.go)), the root names the cloud ones ([`cmd/problemkind.go`](../cmd/problemkind.go)), and `cliout` names `usage` and `input_required`. The root composes the tables, `cmd/local`'s first, and the first match wins.

| kind | meaning |
| --- | --- |
| `usage` | the command was invoked wrongly; exit status 2 |
| `input_required` | the command needed an answer it would have asked for, and this run could not ask: under `--output json`, or without a terminal where the command refuses to guess. The message names the question and the flag that answers it. Exit status 1 (see [Prompts](#prompts)) |
| `no_project` | no project here: no `pyproject.toml` with `[tool.astro]` |
| `foreign_mode` | the project is running in the other mode |
| `already_running` | the project is already running |
| `health_timeout` | Airflow did not become healthy in time |
| `locked` | another start of this project is in progress; try again (Unix only) |
| `unsupported_base` | the image base is not one this mode can run |
| `database_newer_than_airflow` | the metadata database was migrated by a newer Airflow (Docker mode) |
| `not_running` | nothing is running for this project |
| `deployment_hibernating`, `deployment_deploying`, `deployment_unhealthy`, `airflow_unavailable` | an Astro Deployment's Airflow did not answer (see [instances.md](instances.md#when-a-deployments-airflow-does-not-answer)); `deployment_deploying` and `airflow_unavailable` mean try again |
| `unauthenticated` | no usable Astro login: none was made, the API refused the token (401), or one was needed and a run under `--output json` may not start a browser login |
| `forbidden` | the login lacks the permission this needs (403) |
| `not_found` | the Astro API has no such object (404) |
| `conflict` | the Astro API refused because of the object's current state, such as one that already exists (409) |
| `api_unavailable` | the Astro API failed on its side (5xx); try again |

The cloud kinds come from the status on the `*httputil.StatusError` that every Astro API failure is normalized into, and `unauthenticated` also from the "no context" and "not logged in" errors raised before any request. APC (Houston) failures carry no kind: its client reports them as text.

A failure with no kind publishes no `kind` key rather than an `unknown` catch-all. A kind is added only when the CLI can recognise the failure reliably (a sentinel it wraps, a type it can assert, or a status on one), never for a failure nothing emits. Changing a kind breaks whatever reads it.

### Removed 1.x commands and flags

A script written for Astro CLI 1.x is told what replaced what it typed rather than only that it is unknown. Either way the failure is a usage error: exit 2, and the `usage` error object under `--output json`.

A removed command stays in the tree as a hidden stub that fails naming its replacement: `astro dev` ([`cmd/local/dev.go`](../cmd/local/dev.go)), `astro run`, `astro deployment airflow-variable|connection|pool`, and `astro env … create|update`.

A removed flag is not registered on any command. Two files in `cmd/` describe it:

- [`cmd/v1_flags.tsv`](../cmd/v1_flags.tsv) is the 1.x inventory, embedded in the binary: every visible flag on every runnable 1.x command, its own and those it inherited, with its shorthand and whether it took a value. It records three 1.x trees: Astro for a non-hosted organization (`astro`), Astro for a hosted one (`astro-hosted`, whose `deployment create` and `update` had flags of their own), and APC (`apc`). It records 1.x, so nothing in v2 changes it.
- `removedFlags` in [`cmd/removed_flags.go`](../cmd/removed_flags.go) is the registry of messages, keyed by flag name.

The root's flag error func (`flagError` in [`cmd/unknown.go`](../cmd/unknown.go)) runs when cobra reports a flag the invoked command does not have. It records the flag in telemetry like any unknown flag. Then, if the inventory says 1.x had that flag (or that shorthand) on that command (at its v2 path, or at a 1.x name it keeps as an alias) in the tree this machine's scripts were written against (`v1TreeOf`: APC, or Astro hosted or not, read from the context only then), it returns the message of the first registry entry that applies there, as a usage error. This happens while flags parse, so it fails before any pre-run logs in or asks an API anything. A command that still has a flag of that name never gets there. Neither does one 1.x did not have, or had without the flag: `astro local reset -f` gets cobra's own error, because 1.x had no `local reset`. A registry entry can narrow itself further. `needs` limits it to commands that have the replacement (`--json` maps to `-o json` only where there is an `-o`), and `under` limits it to a command and everything below it, for a name whose replacement differs by command (`--template` on `deployment inspect`). Where names overlap, the narrower entry comes first.

A run that asks for help (`-h`, `--help` or `--help=true`) gets the help, not the refusal. A shell asking for completions (`__complete`) gets them: cobra parses flags itself there, without the flag error func, so when a flag typed is one the command being completed does not have, `acceptRemovedFlags` gives that command each removed flag 1.x had on it as a hidden flag, for that run only. The command completed is the one cobra's own `Find` resolves, so a removed flag typed before a subcommand's name (`astro api airflow --json ls`) completes as cobra would complete it without the tombstones: a run of that line fails too, since the parent never had the flag.

To remove a flag, delete it and add an entry for its name if none applies yet. `TestEveryV1FlagStillWorksOrSaysWhatReplacedIt` runs every inventory flag against its v2 tree. It fails on any flag v2 neither has nor reports, checks that each refusal is the message its entry gives there, and fails on an entry that no flag in the inventory reaches. `TestRemovedFlagsSayWhatReplacedThem` needs a case for every entry. The entries go in v3, once the unknown-flag events show nobody still passes them.

## Help

Every `--help` page is drawn by one renderer, [`cmd/help.go`](../cmd/help.go), which the root installs with `SetHelpFunc` and `SetUsageFunc` and every command inherits. It wraps prose and flag descriptions to the terminal (at most 100 columns, 80 when piped), lists each command with its aliases, and puts examples after the flags they use. It shows the current context only on commands that run the platform pre-run, so not on the offline core tree. A command does not set a template of its own: the inherited func would ignore it. A section a command needs belongs in the renderer instead, as flag groups do. Annotate a flag with `group` to list it under `<Group> Flags:`, and set `flag-groups` on the command to order those sections (`astro deploy` does both).

What goes into a page is checked by `TestHelpFollowsTheStyleGuide` in [`cmd/help_lint_test.go`](../cmd/help_lint_test.go). It reads the same trees every tree-wide test in `cmd` does (aliases, groups, shorthands, `--output`, the error contract): `rootsUnderTest` in [`cmd/roots_test.go`](../cmd/roots_test.go) builds one for each configuration in its `treeConfigs` table, setting up each one's config immediately before building it. They are Astro for a non-hosted organization and for a hosted one, whose `deployment create` and `update` gain flags of their own, and APC under an APC context at three platform versions: the newest any gate in `cmd/apc` asks for (where `deployment adopt`, `create --mode` and `deploy --dags` appear), one below every gate (where the pre-1.0.0 examples do), and 1.0.0 between them. Between the newest and oldest, both sides of every version gate are built. Lists that excuse commands are keyed by platform and apply to each of its trees. A test that runs a tree rather than reading it uses `treesToExecute`, the two whose surrounding state (`cmd/apc`'s package-level platform version, the config) is still theirs once all five are built. `TestAPCTreesStraddleEveryGate` parses `cmd/apc` for every `houston.VersionRestrictions` literal (the command-availability table's entries among them) and every `VerifyVersionMatch` call, and fails when a gate falls outside them, is not a `GTE`, or gives its version as anything but a literal it can read.

- A command's `Short`, and a flag's description, start with a capital and do not end in a period.
- A top-level command's `Short` fits on its row of the root page at 100 columns. The room is what the renderer leaves after the command column, which the widest spellings size (`deployment, de, deployments`), so it moves when they do.
- A positional argument in `Use` is `<UPPER_SNAKE>` when required and `[UPPER_SNAKE]` when optional, with `...` for one that repeats, as in `astro af runs get <DAG_ID> <RUN_ID>`. Arguments handed on as they are after `--` are spelled `[-- COMMAND...]` (any UPPER_SNAKE name), last, as in `astro link add [NAME] [-- COMMAND...]`.
- A runnable leaf command has an `Example`. Examples are command lines indented two spaces with no `$ ` prompt, so they paste as they are, and any explanation goes on a `  # comment` line above the command. A command line may set environment variables before `astro` (`ASTRO_LOCAL_HEALTH_TIMEOUT=10m astro local start`).
- An example line is at most 100 columns, since examples print as written and are never wrapped. A longer command ends its line with ` \` and continues on a line indented four spaces. Nothing may follow the backslash: with a space after it, a shell runs the next line as a command of its own. A line indented four spaces is a continuation, so it must follow a line ending in ` \`, and a line ending in ` \` must be followed by one.
- Every `astro …` line in an example names a command that exists in that tree, with flags it accepts and an argument count its `Args` validator allows. Nothing runs; placeholders such as `<DEPLOYMENT_ID>` count as arguments, a line continued with ` \` is joined first, leading `NAME=value` words are skipped, and every command on a line that runs astro is checked, after `|`, `||`, `&&` or `;` as well as at its start.
- A paragraph of a `Long` is one line, with a blank line before the next: the renderer wraps it, so a paragraph broken by hand renders with a short line in mid-sentence. A list of `- ` items, or an indented literal, starts after a blank line or a heading ending in `:`, and list items follow one another directly; any other line directly after another is a paragraph broken by hand, including one sentence per line. The renderer hangs a list item's continuation itself, so an item is one line too. Indented lines are left as written.
- A `Short`, a `Long`, an example or a flag's description writes a command, flag or path as plain text, with no backticks (see [Commands in messages](#commands-in-messages)); `TestHelpHasNoBackticks` fails on a page that prints one.

The commands that failed a rule when it was written are listed per rule in [`cmd/help_exceptions_test.go`](../cmd/help_exceptions_test.go). That list only shrinks: fix a command's help and delete its line, and `TestHelpExceptionsOnlyShrink` fails on a line left behind. A new command meets the rules rather than joining the list. Every list is empty today.

## Local state

| path | what | lifetime |
| --- | --- | --- |
| `<project>/pyproject.toml` | the manifest, parsed by `pkg/manifest` | authored, committed |
| `<project>/.venv/` | the project environment | derived, gitignored |
| `<project>/.env` | plain project values (see [secrets.md](secrets.md)) | per-machine, gitignored |
| `<project>/.astro/standalone/` | a standalone Airflow's `AIRFLOW_HOME`: metadata database, logs, generated admin password | per-machine, gitignored |
| `~/.astro/` | durable per-user state: auth and config (`config.yaml`), the vault (`secrets/`), the proxy (`proxy/`) | durable |
| `~/.cache/astro/projects/<id>/` | rebuildable runtime state (`$XDG_CACHE_HOME/astro` when set) | rebuildable |

A project's id is the sha256 of its canonical path (`localrt.CanonicalPath`): absolute, symlinks resolved, and each component spelled the way the filesystem spells it, so differently-cased spellings of one directory are one project. Hostnames are display labels, not keys.

The project's cache directory holds two files that never merge:

- `runtime.json`, the record of a running Airflow, owned by `pkg/localrt` and shared by both engines. Fields: `projectPath`, `mode`, `pid`/`pgid` (standalone: the supervisor and its process group; signals and liveness address the group, because Airflow's components outlive the supervisor during shutdown) or `composeProject` (docker, plus the starter's `pid` with stop-with-session), `port`, `hostname`, `airflowMajor`, `startedAt`, `stopWithSession`. It is v2-only, so fields can be added freely.
- `state.json`, per-user preferences (`internal/userstate`), such as the `astro use` selection. userstate rewrites it canonically and would drop foreign fields, which is why the two are separate.

Tools coordinate through disk, not through each other: any of them can reconnect to, inspect or stop an Airflow another started. Whether an Airflow outlives its starter is `StopWithSession` on the plan: false by default (it keeps running), true to stop it when the starting process exits.

### Route pruning across owners

`pkg/proxy` prunes `routes.json` on every write, and two owners write it: the CLI and Astro Desktop. A route's stored PID and its owner's live PID drift apart when the owner restarts, so a plain PID check would let one owner drop a route whose Airflow is still running.

The rule: **a route is never evicted while its runtime record reports the runtime alive by that mode's own liveness check.** `pkg/localrt` builds its proxy stores with this predicate (`proxy.WithRouteLiveness`), resolving each route to its record by project path:

- Standalone: kept while the recorded process group (`pgid`, else `pid`) is alive.
- Docker: never pruned by PID, since containers outlive their starter. A docker route is removed only by a stop or by `astro local list --clean`, which checks real compose state.
- No matching record (a route a 1.x CLI wrote, or a project directory that is gone): the route's own PID check, with docker routes kept.

The predicate arrives from outside, so `pkg/proxy` has no astro-cli imports. It costs a record read and a signal, no container-engine call. Stores built outside `pkg/localrt` (the proxy daemon itself, Otto's config) use the default PID check.

### The proxy port and astro 1.x

The proxy serves on 6563. astro 1.x runs its own proxy (`astro dev proxy serve`) on 6563 from the same `~/.astro/proxy` directory and `routes.json`, so one proxy serves the projects of both. When the CLI's daemon starts and a 1.x proxy holds 6563, the daemon stops that proxy and serves there itself. It stops a process only when that process runs `dev proxy serve` for 6563, reads the same route store (from its `ASTRO_HOME` or `HOME`), and answers like an astro proxy; never Astro Desktop's proxy, its own daemon, or a process it cannot identify. In that last case the daemon falls back to another port, saves it in `proxy.fallback-port`, and tries it first on the next start, so URLs stay stable. A 1.x CLI that later finds this daemon's record stops it and takes 6563 back, and the next v2 start does the same in return; each set of projects stays reachable on 6563 either way.

### Listing and cleanup

`astro local list` enumerates every record under the cache root, reports each through its engine's liveness (docker asks the container engine, standalone checks the group), and joins live routes for a hostname. A record with no live runtime is shown as `stopped (stale)` with `--all`, and hidden by default. `astro local list --clean` removes every stale record and its route, machine-wide. It lives on `list` rather than `reset` because `reset` acts on one project (stop plus wipe).

### Compose overrides

Docker mode reads a project's `docker-compose.override.yml` (not the `.yaml` spelling) and passes it to compose as a second `--file` after the file it generates, so compose merges the two by its own rules and resolves `${VAR}` from the shell and the project's `.env`. The names `astro dev` used are kept, so a 1.x project's override still works: the `airflow` network, and the services `postgres`, `scheduler`, `triggerer`, `dag-processor` and `api-server` (Airflow 3) or `webserver` (Airflow 2). A service with `profiles:` does not start unless the shell sets `COMPOSE_PROFILES`. Stop and reset address the compose project by name, so they take the override's containers, networks and (on reset) volumes down too. Standalone mode runs no containers: it ignores the file and warns once at start.

## Platform support

Windows runs Docker mode only. Standalone mode and the proxy daemon are macOS and Linux.

## Dev-mode Airflow defaults

A local Airflow is configured for someone developing DAGs, in both modes unless noted:

- **DAGs are created unpaused but run only when triggered.** `AIRFLOW__CORE__DAGS_ARE_PAUSED_AT_CREATION=False` and `AIRFLOW__SCHEDULER__USE_JOB_SCHEDULE=False`: the scheduler creates no runs of its own, so nothing runs on a cron clock and asset-triggered DAGs do not cascade, while a run triggered from the UI, `astro local af runs trigger` or the API starts at once. Paused-at-creation protects a shared scheduler from a DAG nobody reviewed; locally the author is watching, and a triggered run on a paused DAG only sits queued. `astro local start` and `restart` say schedules are off, and how to turn them on, unless the project already did.
- **Fast DAG rescan.** Each file is re-parsed every 3 seconds (`MIN_FILE_PROCESS_INTERVAL=3`) and the folder listed every 2, so a saved change shows within about 5 seconds at 1–2% of a core.
- **Zero default task retries** (standalone mode).
- **Loopback only.** Standalone binds the API server and webserver to 127.0.0.1; Docker mode publishes its ports on 127.0.0.1.

A project that sets one of these keys itself, in its `.env` or vault (or, in standalone, its shell), keeps its own value. The loopback bind is the exception, because it guards the instance rather than states a preference.
