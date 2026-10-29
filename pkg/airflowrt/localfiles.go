package airflowrt

import "slices"

// PickleFixRule is the one per-machine path outside .astro/: the LocalExecutor
// pickling fix the standalone engine drops into plugins/ for Airflow 2 on
// macOS (AF2PickleFixPlugin).
const PickleFixRule = "plugins/fix_local_executor_pickle.py"

// localFileRules are the per-machine files Astro tools write into a project,
// all but one under .astro/, as .gitignore/.dockerignore rules. Each holds
// something true of one machine and wrong, or secret, on any other:
//
//   - .astro/standalone/ is a standalone Airflow's AIRFLOW_HOME
//     (StandaloneDir): its SQLite database, its logs, and the generated admin
//     password.
//   - .astro/worktrees/ holds the git worktrees Astro Desktop nests inside a
//     project, each pointing back at this clone by absolute path.
//   - .astro/*.local.yaml and .astro/*.local.yml cover the standalone af CLI's
//     instance file, config.local.yaml: a local URL derived from this machine's
//     directory name and proxy port, the local admin credentials, and the
//     current instance. .astro/*.local.lock covers the lock file taken beside
//     it while it is written, config.local.lock: empty, and meaningless on any
//     other machine, so it belongs in no repository and no image either.
//   - .astro/otto/*.local.json covers Otto's per-machine permissions scope,
//     permissions.local.json. Its sibling permissions.json is meant to be
//     committed, and so is extensions.json, which is why .astro/otto/ is not
//     ignored as a whole.
//   - .astro/otto/mcp.json carries Astro Desktop's loopback MCP address and a
//     bearer token for it. Otto's MCP loader fixes the name, so the *.local.json
//     rule cannot cover it.
//   - PickleFixRule is machine-specific and recreated on the next start
//     whenever it is missing, so it has no business in the repo or an image.
//     Only that file: the rest of plugins/ is the project's own code.
//
// Nothing here covers .astro/ itself: .astro/config.yaml holds deployment
// links and .astro/memory/ is Otto's shared project memory, and both are
// committed.
var localFileRules = []string{
	StandaloneDir + "/",
	".astro/worktrees/",
	".astro/*.local.yaml",
	".astro/*.local.yml",
	".astro/*.local.lock",
	".astro/otto/*.local.json",
	".astro/otto/mcp.json",
	PickleFixRule,
}

// LocalFileRules returns the per-machine file rules, one list for every place
// that keeps them out: the .gitignore and .dockerignore pkg/scaffold writes,
// and the files a generated image leaves out (pkg/imagebuild). A copy, so no
// caller can change what the others read.
func LocalFileRules() []string {
	return slices.Clone(localFileRules)
}

// shippedImageRules are files no image a project ships may carry, whoever
// wrote them, as .dockerignore rules. They differ from localFileRules in one
// way that matters: they are not added to .gitignore, since some (an
// airflow.cfg, a .env.example) are committed on purpose, and git is the
// project's business. An image is pushed to a registry, and none of these
// belongs in one:
//
//   - **/.env, **/.env.* and **/.envrc, at any depth: local environment
//     values (`astro local env set --plain` writes .env), their per-stage
//     variants, and direnv's file of exported variables. The variants include
//     templates such as .env.example, which have no use inside an image either.
//   - **/airflow_settings.yaml, at any depth: the 1.x CLI's local connections,
//     variables and pools, with their values in cleartext. Nothing on Astro
//     reads it.
//   - airflow.db, airflow.cfg and logs/, at the root only: what an Airflow run
//     with the project as its AIRFLOW_HOME leaves there, a SQLite database with
//     its connections, a config with its fernet key and database URL, and its
//     task logs. The image's AIRFLOW_HOME is the project's root, so a shipped
//     airflow.cfg would replace the runtime's own. Below the root these names
//     are the project's own data, and are left alone.
//
// 1.x's default .dockerignore listed these too, except the .env variants and
// .envrc, plus `astro`, the CLI binary some installs dropped at the root. That
// one is not here: a root astro/ can be a Python package (astro-sdk imports
// as `astro`), and leaving it out would break a DAG's import without a word.
var shippedImageRules = []string{
	"**/.env",
	"**/.env.*",
	"**/.envrc",
	"**/airflow_settings.yaml",
	"airflow.db",
	"airflow.cfg",
	"logs/",
}

// ShippedImageRules returns the rules every shipped image applies beyond the
// per-machine ones (LocalFileRules): the files generated images leave out
// (pkg/imagebuild), and that pkg/scaffold writes into the .dockerignore a
// declared Dockerfile's build reads. A copy, so no caller can change what the
// others read.
func ShippedImageRules() []string {
	return slices.Clone(shippedImageRules)
}
