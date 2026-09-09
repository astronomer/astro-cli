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

// A file stat is reported at a path that resolves inside the project.
//
// dagbag_stats names each file relative to the DAGs FOLDER — and some Airflow
// versions put a separator in front of that — while dag.fileloc is absolute.
// Relativizing a stat's path as if it were absolute resolved it against the
// running process's working directory instead, so the reported path depended on
// who invoked the check and pointed outside the project from anywhere but the
// project root. From the project root it still dropped the DAGs folder, naming a
// file that is not there.
//
// It matters because the duplicate-dag_id and slow-parse findings carry these
// paths and nothing else identifies the file: a user is told which file to go
// fix, so the path has to be one they can open.
//
// The real script runs under a real interpreter here, against a stub airflow
// package that returns the two stat shapes. That is what makes this a test of
// the script rather than of a re-implementation of it; Airflow itself is not
// installed in CI, and installing it to assert a path would be a heavy way to
// control one field.
func TestFileStatsAreReportedRelativeToTheProject(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not on PATH; skipping the embedded-script integration test")
	}

	project := t.TempDir()
	dags := filepath.Join(project, "dags")
	require.NoError(t, os.MkdirAll(filepath.Join(dags, "nested"), 0o755))

	report, err := (&VenvRunner{
		Exec:       execExecutor{},
		pythonPath: func(string) string { return python },
		tempHome:   defaultTempHome,
	}).Parse(context.Background(), ParseInput{
		ProjectPath: project,
		DagsDir:     dags,
		Env:         append(os.Environ(), "PYTHONPATH="+stubAirflow(t)),
	})
	require.NoError(t, err)
	require.Empty(t, report.Fatal, "the stub airflow package should import cleanly")

	byFile := map[string]ReportFile{}
	for _, f := range report.Files {
		byFile[f.File] = f
	}
	// Both stat shapes land under dags/, with the folder the stat omitted.
	assert.Contains(t, byFile, filepath.Join("dags", "plain.py"),
		"a DAGs-folder-relative stat must be reported under the DAGs folder")
	assert.Contains(t, byFile, filepath.Join("dags", "nested", "slashed.py"),
		"a stat prefixed with a separator is still DAGs-folder-relative, not absolute")

	for _, f := range report.Files {
		assert.False(t, filepath.IsAbs(f.File), "%q should be project-relative", f.File)
		assert.NotContains(t, f.File, "..", "%q escapes the project", f.File)
	}

	// The absolute dag.fileloc path keeps working: it must not be re-rooted
	// under the DAGs folder a second time.
	require.Len(t, report.Dags, 1)
	assert.Equal(t, filepath.Join("dags", "plain.py"), report.Dags[0].File)
}

// stubAirflow writes the smallest airflow package the parse program can build a
// DagBag from, and returns the directory to put on PYTHONPATH. The DagBag
// reports one stat per shape the real one is known to produce.
func stubAirflow(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	pkg := filepath.Join(root, "airflow", "models")
	require.NoError(t, os.MkdirAll(pkg, 0o755))

	write := func(path, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	write(filepath.Join(root, "airflow", "__init__.py"), "")
	write(filepath.Join(root, "airflow", "models", "__init__.py"), "")
	// dag_folder is passed as an absolute path; the stats deliberately are not.
	write(filepath.Join(pkg, "dagbag.py"), `import datetime, os


class _Stat:
    def __init__(self, file, dag_ids):
        self.file = file
        self.dags = dag_ids
        self.duration = datetime.timedelta(seconds=0.5)


class _Dag:
    def __init__(self, fileloc):
        self.fileloc = fileloc


class DagBag:
    def __init__(self, dag_folder=None, include_examples=False, **kwargs):
        folder = dag_folder or os.getcwd()
        self.import_errors = {}
        # Absolute, the way the real dag.fileloc is.
        self.dags = {"only": _Dag(os.path.join(folder, "plain.py"))}
        self.dagbag_stats = [
            # Relative to the DAGs folder, the common shape.
            _Stat("plain.py", ["only"]),
            # Same thing with a leading separator, which older Airflow emits
            # after stripping the folder prefix by string replacement.
            _Stat(os.sep + os.path.join("nested", "slashed.py"), []),
        ]
`)
	return root
}
