package checks

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Airflow 3.3 removed DagBag's include_examples, and passing it there raises a
// TypeError before a single DAG is read, so every check on a 3.3 project failed
// as "not ready to check". Older DagBags still take it and default it to the
// load_examples setting, where leaving it out would check Airflow's example
// DAGs as if they were the project's. The script passes it only where it is a
// parameter, and these stubs are the two shapes.
func TestParseBuildsADagBagWithAndWithoutIncludeExamples(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not on PATH; skipping the embedded-script integration test")
	}

	for _, tc := range []struct {
		name string
		init string
	}{
		{
			name: "3.3, which has no include_examples",
			init: "def __init__(self, dag_folder=None, safe_mode=True, load_op_links=True):\n" +
				"        self.dags = {'mine': _Dag(os.path.join(dag_folder, 'mine.py'))}\n",
		},
		{
			name: "before 3.3, where it defaults to loading the examples",
			init: "def __init__(self, dag_folder=None, include_examples=None, **kwargs):\n" +
				"        self.dags = {'mine': _Dag(os.path.join(dag_folder, 'mine.py'))}\n" +
				"        if include_examples is not False:\n" +
				"            self.dags['example_bash_operator'] = _Dag('/airflow/example_dags/example_bash_operator.py')\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project := t.TempDir()
			dags := filepath.Join(project, "dags")
			require.NoError(t, os.MkdirAll(dags, 0o755))

			report, err := (&VenvRunner{
				Exec:       execExecutor{},
				pythonPath: func(string) string { return python },
				tempHome:   defaultTempHome,
			}).Parse(context.Background(), ParseInput{
				ProjectPath: project,
				DagsDir:     dags,
				Env:         append(os.Environ(), "PYTHONPATH="+stubDagBag(t, tc.init)),
			})
			require.NoError(t, err)
			require.Empty(t, report.Fatal)
			require.Len(t, report.Dags, 1, "the project's DAG and nothing else: %+v", report.Dags)
			assert.Equal(t, "mine", report.Dags[0].DagID)
		})
	}
}

// stubDagBag writes an airflow package whose DagBag has the given __init__, and
// returns the directory to put on PYTHONPATH.
func stubDagBag(t *testing.T, init string) string {
	t.Helper()
	root := t.TempDir()
	pkg := filepath.Join(root, "airflow", "models")
	require.NoError(t, os.MkdirAll(pkg, 0o755))
	for path, body := range map[string]string{
		filepath.Join(root, "airflow", "__init__.py"): "",
		filepath.Join(pkg, "__init__.py"):             "",
		filepath.Join(pkg, "dagbag.py"): "import os\n\n\n" +
			"class _Dag:\n    def __init__(self, fileloc):\n        self.fileloc = fileloc\n\n\n" +
			"class DagBag:\n    import_errors = {}\n    dagbag_stats = []\n\n    " + init,
	} {
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	return root
}
