//go:build !windows

package settings

import (
	"os"
	"os/exec"
	"path/filepath"
)

// The export script (exportScript) run in bash, as both modes run it, against
// a fake airflow on PATH: the file it writes is owner-only, under
// AIRFLOW_HOME rather than the working directory (the project, for a
// standalone one), and gone however the script ends, an interrupt included.
func (s *Suite) TestExportScriptRemovesItsFile() {
	// fakeAirflowBin is an airflow that writes the file it is given (with the
	// umask it ran under, to check), then does what behavior says.
	fakeAirflowBin := func(behavior string) string {
		bin := s.T().TempDir()
		script := "#!/bin/bash\n" +
			`f="$3"` + "\n" + // airflow {connections,variables} export FILE ...
			`echo "umask=$(umask)" > "$f"` + "\n" +
			behavior + "\n"
		s.Require().NoError(os.WriteFile(filepath.Join(bin, "airflow"), []byte(script), 0o700)) //nolint:gosec
		return bin
	}
	// run runs the connections export script in bash, with home as
	// AIRFLOW_HOME and project as its working directory, as standalone does.
	run := func(behavior string) (out, home, project string, err error) {
		home, project = s.T().TempDir(), s.T().TempDir()
		bin := fakeAirflowBin(behavior)
		prev := SetExecAirflowCommand(func(_, command string) (string, error) {
			cmd := exec.Command("bash", "-c", command)
			cmd.Dir = project
			cmd.Env = append(os.Environ(), "AIRFLOW_HOME="+home, "PATH="+bin+":"+os.Getenv("PATH"))
			b, err := cmd.CombinedOutput()
			return string(b), err
		})
		s.T().Cleanup(func() { SetExecAirflowCommand(prev) })
		out, err = readAirflowExport("id", airflowConnExport, "connection")
		return out, home, project, err
	}
	empty := func(dir string) {
		entries, err := os.ReadDir(dir)
		s.Require().NoError(err)
		s.Empty(entries, "%s holds an export file", dir)
	}

	s.Run("read back, owner-only, and removed after a successful export", func() {
		out, home, project, err := run(`echo "connections successfully exported"`)
		s.Require().NoError(err)
		s.Equal("umask=0077\n", out, "the export wrote under a umask other than 077")
		empty(home)
		empty(project)
	})

	for name, behavior := range map[string]string{
		"an export that fails after writing": `exit 1`,
		"an interrupt":                       `kill -INT $PPID; sleep 1; echo "connections successfully exported"`,
		"a termination":                      `kill -TERM $PPID; sleep 1; echo "connections successfully exported"`,
	} {
		s.Run("removed after "+name, func() {
			_, home, project, err := run(behavior)
			s.Error(err)
			empty(home)
			empty(project)
		})
	}
}
