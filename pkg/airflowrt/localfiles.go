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
//     current instance.
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
