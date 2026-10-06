package local

import (
	"io"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/apirequest"
	"github.com/astronomer/astro-cli/internal/localenv"
)

// A list under `--output json` is one object holding its rows under a named
// key, `{"dags": [...]}`, and the key is [] when there are none, never null
// and never missing. NDJSON is for streams (logs, events, build progress,
// `local start`, `local check`), which a reader consumes as they arrive; a
// list is finished before it is printed, and one object is what `jq '.dags[]'`
// and a JSON parser expect.
//
// The keys are the standalone `af` CLI's (astro-airflow-mcp), and so is the
// envelope around them where it has one: `total_<key>` is how many the
// collection holds, `returned_count` how many this page carries. `astro af`
// stands in for that `af`, and the skills and agents written against it
// (`af runs list | jq '.dag_runs[]'`, Otto reading `af dags errors` as
// total_import_errors and import_errors) then read this one unchanged. A list
// with no af counterpart takes a plain snake_case plural.
//
// One named type per list rather than one generic type with a computed key:
// the key is then a struct tag, a golden in testdata/schema pins each
// envelope with its key, and the emit observer in the tests sees the type.

// emitRows renders a list: in json mode the one object envelope builds from
// the rows and the collection's total, in text mode the text renderer over
// the same rows. rows is made non-nil here, once, so no list can publish null.
//
// It is the only list renderer, and it has no streaming branch on purpose: a
// list cannot be emitted as one object per line from here.
func emitRows[Row, Env any](r cliout.Renderer, rows []Row, total int,
	envelope func(rows []Row, total int) Env, text func(io.Writer, []Row) error,
) error {
	if rows == nil {
		rows = []Row{}
	}
	return r.Emit(envelope(rows, total), func(w io.Writer) error { return text(w, rows) })
}

// dagList is `astro af dags list`, af's `dags list`.
type dagList struct {
	Total    int      `json:"total_dags"`
	Returned int      `json:"returned_count"`
	DAGs     []dagRow `json:"dags"`
}

func newDAGList(rows []dagRow, total int) dagList { return dagList{total, len(rows), rows} }

// dagStatList is `astro af dags stats`. af prints Airflow's own dagStats
// response, `{"dags": [...], "total_entries": n}`, so the key and the count
// are Airflow's names, and so is a row's shape; see dagStatRow.
type dagStatList struct {
	DAGs  []dagStatRow `json:"dags"`
	Total int          `json:"total_entries"`
}

func newDAGStatList(rows []dagStatRow, total int) dagStatList { return dagStatList{rows, total} }

// importErrorList is `astro af dags errors`, af's `dags errors`.
type importErrorList struct {
	Total        int              `json:"total_import_errors"`
	Returned     int              `json:"returned_count"`
	ImportErrors []importErrorRow `json:"import_errors"`
}

func newImportErrorList(rows []importErrorRow, total int) importErrorList {
	return importErrorList{total, len(rows), rows}
}

// dagWarningList is `astro af dags warnings`, af's `dags warnings`.
type dagWarningList struct {
	Total       int             `json:"total_dag_warnings"`
	Returned    int             `json:"returned_count"`
	DAGWarnings []dagWarningRow `json:"dag_warnings"`
}

func newDAGWarningList(rows []dagWarningRow, total int) dagWarningList {
	return dagWarningList{total, len(rows), rows}
}

// runList is `astro af runs list`, af's `runs list`.
type runList struct {
	Total    int      `json:"total_dag_runs"`
	Returned int      `json:"returned_count"`
	DAGRuns  []runRow `json:"dag_runs"`
}

func newRunList(rows []runRow, total int) runList { return runList{total, len(rows), rows} }

// taskInstanceList is `astro af runs tasks`. af has no such command; the key
// is the one Airflow's own task-instance listing uses, in af's envelope.
type taskInstanceList struct {
	Total         int               `json:"total_task_instances"`
	Returned      int               `json:"returned_count"`
	TaskInstances []taskInstanceRow `json:"task_instances"`
}

func newTaskInstanceList(rows []taskInstanceRow, total int) taskInstanceList {
	return taskInstanceList{total, len(rows), rows}
}

// taskList is `astro af tasks list`, af's `tasks list`.
type taskList struct {
	Total    int       `json:"total_tasks"`
	Returned int       `json:"returned_count"`
	Tasks    []taskRow `json:"tasks"`
}

func newTaskList(rows []taskRow, total int) taskList { return taskList{total, len(rows), rows} }

// assetList is `astro af assets list`, af's `assets list`.
type assetList struct {
	Total    int        `json:"total_assets"`
	Returned int        `json:"returned_count"`
	Assets   []assetRow `json:"assets"`
}

func newAssetList(rows []assetRow, total int) assetList { return assetList{total, len(rows), rows} }

// assetEventList is `astro af assets events`, af's `assets events`.
type assetEventList struct {
	Total       int             `json:"total_asset_events"`
	Returned    int             `json:"returned_count"`
	AssetEvents []assetEventRow `json:"asset_events"`
}

func newAssetEventList(rows []assetEventRow, total int) assetEventList {
	return assetEventList{total, len(rows), rows}
}

// runTriggers is `astro af assets triggers`, in af's `assets triggers` shape:
// the run it is about, and the events that started it.
type runTriggers struct {
	DAGID      string          `json:"dag_id"`
	RunID      string          `json:"dag_run_id"`
	Events     []assetEventRow `json:"triggered_by_events"`
	EventCount int             `json:"event_count"`
}

// connectionList is `astro af connections list`, af's `config connections`.
type connectionList struct {
	Total       int                 `json:"total_connections"`
	Returned    int                 `json:"returned_count"`
	Connections []connectionListRow `json:"connections"`
}

func newConnectionList(rows []connectionListRow, total int) connectionList {
	return connectionList{total, len(rows), rows}
}

// variableList is `astro af variables list`, af's `config variables`.
type variableList struct {
	Total     int               `json:"total_variables"`
	Returned  int               `json:"returned_count"`
	Variables []variableListRow `json:"variables"`
}

func newVariableList(rows []variableListRow, total int) variableList {
	return variableList{total, len(rows), rows}
}

// poolList is `astro af pools list`, af's `config pools`.
type poolList struct {
	Total    int       `json:"total_pools"`
	Returned int       `json:"returned_count"`
	Pools    []poolRow `json:"pools"`
}

func newPoolList(rows []poolRow, total int) poolList { return poolList{total, len(rows), rows} }

// providerList is `astro af providers`, af's `config providers`.
type providerList struct {
	Total     int           `json:"total_providers"`
	Returned  int           `json:"returned_count"`
	Providers []providerRow `json:"providers"`
}

func newProviderList(rows []providerRow, total int) providerList {
	return providerList{total, len(rows), rows}
}

// pluginList is `astro af plugins`, af's `config plugins`.
type pluginList struct {
	Total    int         `json:"total_plugins"`
	Returned int         `json:"returned_count"`
	Plugins  []pluginRow `json:"plugins"`
}

func newPluginList(rows []pluginRow, total int) pluginList { return pluginList{total, len(rows), rows} }

// configSectionList is `astro af config`, af's `config show`: Airflow's own
// nesting, one entry per section with its options inside, and af's count of
// the sections beside it.
type configSectionList struct {
	Total    int                `json:"total_sections"`
	Sections []configSectionRow `json:"sections"`
}

func newConfigSectionList(rows []configSectionRow, total int) configSectionList {
	return configSectionList{total, rows}
}

// endpointList is `astro local api ls`. The key and the count are af's `api
// ls`; af's endpoints are bare path strings, these are objects with the path
// under "path".
type endpointList struct {
	Endpoints []apirequest.EndpointRow `json:"endpoints"`
	Count     int                      `json:"count"`
}

func newEndpointList(rows []apirequest.EndpointRow, _ int) endpointList {
	return endpointList{rows, len(rows)}
}

// localList is `astro local list`: the local Airflows, one per project. No af
// counterpart; each row is keyed by its project, so the list is "projects".
type localList struct {
	Projects []listRow `json:"projects"`
}

func newLocalList(rows []listRow, _ int) localList { return localList{rows} }

// localRemoved is `astro local list --clean`: the stale records it removed.
type localRemoved struct {
	Removed []listRow `json:"removed"`
}

func newLocalRemoved(rows []listRow, _ int) localRemoved { return localRemoved{rows} }

// envList is `astro local env list` and its per-kind `env var list` and `env
// conn list`. No af counterpart. A row is a name of either kind, declared or
// found in a file, with where it resolves from and never its value, so the
// list is "entries" rather than "variables" or "values".
type envList struct {
	Entries []localenv.ListItem `json:"entries"`
}

func newEnvList(rows []localenv.ListItem, _ int) envList { return envList{rows} }
