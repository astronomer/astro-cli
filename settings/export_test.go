package settings

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// fakeAirflow answers the script an export runs (exportScript) as it would
// run: Airflow's log, then the marker, then the file. Any other command
// answers from others. exportshell_test.go runs the script itself in bash.
type fakeAirflow struct {
	// variables and connections are what the exports write.
	variables, connections string
	// others answers any other command, by the command.
	others map[string]string
	// fail makes the script fail; unsuccessful makes Airflow's log not say
	// it succeeded.
	fail, unsuccessful bool

	scripts []string
}

var (
	errFake       = errors.New("fake failure")
	markerPattern = regexp.MustCompile(`echo (ASTRO-EXPORT-\S+)`)
)

// install makes f the Airflow the package runs commands against, for the
// rest of the test.
func (s *Suite) install(f *fakeAirflow) *fakeAirflow {
	prev := SetExecAirflowCommand(f.run)
	s.T().Cleanup(func() { SetExecAirflowCommand(prev) })
	return f
}

func (f *fakeAirflow) run(_, command string) (string, error) {
	var content string
	switch {
	case strings.Contains(command, "airflow variables export "):
		content = f.variables
	case strings.Contains(command, "airflow connections export "):
		content = f.connections
	default:
		return f.others[command], nil
	}
	f.scripts = append(f.scripts, command)
	if f.fail {
		return "", errFake
	}
	log := "exports successfully written\n"
	if f.unsuccessful {
		log = "nothing to say\n"
	}
	return log + markerPattern.FindStringSubmatch(command)[1] + "\n" + content, nil
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
	// docker exec -t ends lines with \r\n.
	fakeConnections = "local_postgres=postgres://username:password@example.db.example.com:5432/schema\r\nother=http://example.com\r\n"
	fakeVariables   = `{"myvar": "myval", "count": 42, "config": {"a": 1}}`
)

func (s *Suite) TestEnvExportConnections() {
	s.Run("writes each connection, with no \\r from a terminal", func() {
		s.install(&fakeAirflow{connections: fakeConnections})
		env, read := s.envFile()

		s.Require().NoError(EnvExportConnections("id", env))
		s.Equal("\nAIRFLOW_CONN_LOCAL_POSTGRES=postgres://username:password@example.db.example.com:5432/schema\nAIRFLOW_CONN_OTHER=http://example.com", read())
	})

	s.Run("each export has a file of its own", func() {
		f := s.install(&fakeAirflow{connections: fakeConnections})
		env, _ := s.envFile()

		s.Require().NoError(EnvExportConnections("id", env))
		s.Require().NoError(EnvExportConnections("id", env))
		s.Require().Len(f.scripts, 2)
		s.NotEqual(f.scripts[0], f.scripts[1])
	})

	for name, f := range map[string]*fakeAirflow{
		"a script that fails":                {fail: true},
		"an export that does not say it did": {unsuccessful: true},
	} {
		s.Run(name+" is an error", func() {
			f.connections = fakeConnections
			s.install(f)
			env, _ := s.envFile()
			s.Error(EnvExportConnections("id", env))
		})
	}

	s.Run("a .env that cannot be written is an error", func() {
		s.install(&fakeAirflow{connections: fakeConnections})
		s.ErrorContains(EnvExportConnections("id", filepath.Join(s.T().TempDir(), "missing", "test.env")), "Writing connections to file unsuccessful")
	})
}

func (s *Suite) TestEnvExportVariables() {
	s.Run("writes each variable, a non-string value as its JSON", func() {
		s.install(&fakeAirflow{variables: fakeVariables})
		env, read := s.envFile()

		s.Require().NoError(EnvExportVariables("id", env))
		got := read()
		s.Contains(got, "\nAIRFLOW_VAR_MYVAR=myval")
		s.Contains(got, "\nAIRFLOW_VAR_COUNT=42")
		s.Contains(got, `AIRFLOW_VAR_CONFIG={"a": 1}`)
	})

	s.Run("variables that do not decode are an error, not a .env missing them", func() {
		s.install(&fakeAirflow{variables: "not json"})
		env, _ := s.envFile()
		s.ErrorContains(EnvExportVariables("id", env), "variable json decode unsuccessful")
	})
}

func (s *Suite) TestDecodeVariables() {
	vars, err := decodeVariables(fakeVariables)
	s.Require().NoError(err)
	s.Equal(map[string]string{"myvar": "myval", "count": "42", "config": `{"a": 1}`}, vars)
}
