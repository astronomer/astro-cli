package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// fakeAirflow stands in for the Airflow the export commands run against: an
// export writes the file it is given, `cat` reads it back and `rm -f` removes
// it, so a test can see which files an export leaves behind. Any other
// command answers from others.
type fakeAirflow struct {
	// variables and connections are what the exports write.
	variables, connections string
	// others answers any other command, by the command.
	others map[string]string
	// partialExport makes an export write its file and then fail;
	// unsuccessful makes it succeed without saying so.
	partialExport, unsuccessful bool
	// failCat and failRm make reading and removing the file fail.
	failCat, failRm bool

	files    map[string]string
	exported []string
}

var errFake = errors.New("fake failure")

// install makes f the Airflow the package runs commands against, for the
// rest of the test.
func (s *Suite) install(f *fakeAirflow) *fakeAirflow {
	f.files = map[string]string{}
	prev := SetExecAirflowCommand(f.run)
	s.T().Cleanup(func() { SetExecAirflowCommand(prev) })
	return f
}

func (f *fakeAirflow) run(_, command string) (string, error) {
	fields := strings.Fields(command)
	switch {
	case strings.HasPrefix(command, "airflow variables export "):
		return f.export(fields[3], "variables", f.variables)
	case strings.HasPrefix(command, "airflow connections export "):
		return f.export(fields[3], "connections", f.connections)
	case strings.HasPrefix(command, "cat "):
		if f.failCat {
			return "", errFake
		}
		content, ok := f.files[fields[1]]
		if !ok {
			return "", fmt.Errorf("cat: %s: No such file or directory", fields[1])
		}
		return content, nil
	case strings.HasPrefix(command, "rm -f "):
		if f.failRm {
			return "", errFake
		}
		delete(f.files, fields[2])
		return "", nil
	default:
		return f.others[command], nil
	}
}

func (f *fakeAirflow) export(file, what, content string) (string, error) {
	f.exported = append(f.exported, file)
	f.files[file] = content
	if f.partialExport {
		return "", errFake
	}
	if f.unsuccessful {
		return "nothing to say", nil
	}
	return fmt.Sprintf("%s successfully exported to %s", what, file), nil
}

// left is the export files still there.
func (f *fakeAirflow) left() []string {
	var files []string
	for file := range f.files {
		files = append(files, file)
	}
	return files
}

// envFile is a fresh .env under a temp dir, and reads back what is in it.
func (s *Suite) envFile() (path string, read func() string) {
	path = filepath.Join(s.T().TempDir(), "test.env")
	return path, func() string {
		b, err := os.ReadFile(path)
		s.Require().NoError(err)
		return string(b)
	}
}

const (
	fakeConnections = "local_postgres=postgres://username:password@example.db.example.com:5432/schema\r\nother=http://example.com\r\n"
	fakeVariables   = `{"myvar": "myval"}`
)

// The files an export has Airflow write hold the values in plain text,
// passwords among them, and are removed whatever happens. They are named per
// export and relative, so in the container's working directory or the
// project's Airflow home, never a directory other users share.
func (s *Suite) TestExportsRemoveTheirFiles() {
	s.Run("connections: a file of their own, removed after a successful export", func() {
		f := s.install(&fakeAirflow{connections: fakeConnections})
		env, read := s.envFile()

		s.Require().NoError(EnvExportConnections("id", env))
		s.Require().NoError(EnvExportConnections("id", env))
		s.Empty(f.left())
		s.Require().Len(f.exported, 2)
		s.NotEqual(f.exported[0], f.exported[1], "two exports share a file")
		for _, file := range f.exported {
			s.False(filepath.IsAbs(file), "%s is not relative", file)
			s.NotContains(file, "/", "%s is not in the working directory", file)
		}
		// docker exec -t ends lines with \r\n; none reaches the .env.
		s.Equal(strings.Repeat("\nAIRFLOW_CONN_LOCAL_POSTGRES=postgres://username:password@example.db.example.com:5432/schema\nAIRFLOW_CONN_OTHER=http://example.com", 2), read())
	})

	s.Run("variables: removed after a successful export, to a .env and to settings", func() {
		f := s.install(&fakeAirflow{variables: fakeVariables})
		env, read := s.envFile()

		s.Require().NoError(EnvExportVariables("id", env))
		s.Equal("\nAIRFLOW_VAR_MYVAR=myval", read())
		s.Empty(f.left())

		WorkingPath = "./testfiles/"
		s.Require().NoError(InitSettings("airflow_settings_export.yaml"))
		s.Require().NoError(ExportVariables("id"))
		s.Empty(f.left())
	})

	failures := []struct {
		name string
		fake fakeAirflow
		want string
	}{
		{"an export that fails part-way", fakeAirflow{partialExport: true}, "error exporting connections"},
		{"an export that does not say it succeeded", fakeAirflow{unsuccessful: true}, "connection export unsuccessful"},
		{"a file that cannot be read back", fakeAirflow{failCat: true}, "error reading connections file"},
	}
	for i := range failures {
		tc := failures[i]
		s.Run("connections: removed after "+tc.name, func() {
			tc.fake.connections = fakeConnections
			f := s.install(&tc.fake)
			env, _ := s.envFile()

			err := EnvExportConnections("id", env)
			s.ErrorContains(err, tc.want)
			s.Len(f.exported, 1)
			s.Empty(f.left())
		})
		s.Run("variables: removed after "+tc.name, func() {
			tc.fake.variables = fakeVariables
			f := s.install(&tc.fake)
			env, _ := s.envFile()

			s.Error(EnvExportVariables("id", env))
			s.Empty(f.left())
		})
	}

	s.Run("a file that cannot be removed does not fail an export that worked", func() {
		f := s.install(&fakeAirflow{connections: fakeConnections, failRm: true})
		env, read := s.envFile()

		s.NoError(EnvExportConnections("id", env))
		s.Contains(read(), "AIRFLOW_CONN_LOCAL_POSTGRES=")
		s.Len(f.left(), 1, "the fake keeps a file it failed to remove")
	})

	s.Run("a .env that cannot be written is an error", func() {
		s.install(&fakeAirflow{connections: fakeConnections})
		s.ErrorContains(EnvExportConnections("id", filepath.Join(s.T().TempDir(), "missing", "test.env")), "Writing connections to file unsuccessful")
	})
}
