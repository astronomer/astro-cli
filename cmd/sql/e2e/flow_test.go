package e2e

import (
	"fmt"
	"os"
	"strings"
	"testing"

	sql "github.com/astronomer/astro-cli/cmd/sql"

	"github.com/stretchr/testify/assert"
)

// chdir changes the current working directory to the named directory and
// returns a function that, when called, restores the original working
// directory.
func chdir(t *testing.T, dir string) func() {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	return func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restoring working directory: %v", err)
		}
	}
}

func execFlowCmd(args ...string) error {
	cmd := sql.NewFlowCommand()
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return err
}

func TestE2EFlowCmd(t *testing.T) {
	err := execFlowCmd()
	assert.NoError(t, err)
}

func TestE2EFlowVersionCmd(t *testing.T) {
	err := execFlowCmd("version")
	assert.NoError(t, err)
}

func TestE2EFlowVersionHelpCmd(t *testing.T) {
	err := execFlowCmd("version", "--help")
	assert.NoError(t, err)
}

func TestE2EFlowAboutCmd(t *testing.T) {
	err := execFlowCmd("about")
	assert.NoError(t, err)
}

func TestE2EFlowInitCmd(t *testing.T) {
	projectDir := t.TempDir()
	defer chdir(t, projectDir)()
	err := execFlowCmd("init")
	assert.NoError(t, err)
}

func TestE2EFlowInitCmdInUserHomeDir(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(homeDir, fmt.Sprintf("astro-cli-%s", t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	err = execFlowCmd("init", dir)
	assert.NoError(t, err)
}

func TestE2EFlowInitCmdWithArgs(t *testing.T) {
	cmd := "init"
	testCases := []struct {
		args []string
	}{
		{[]string{cmd, t.TempDir()}},
		{[]string{cmd, t.TempDir(), "--airflow-home", t.TempDir()}},
		{[]string{cmd, t.TempDir(), "--airflow-dags-folder", t.TempDir()}},
		{[]string{cmd, t.TempDir(), "--data-dir", t.TempDir()}},
		{[]string{cmd, t.TempDir(), "--verbose"}},
		{[]string{cmd, t.TempDir(), "--no-verbose"}},
	}
	for _, tc := range testCases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			err := execFlowCmd(tc.args...)
			assert.NoError(t, err)
		})
	}
}

func TestE2EFlowValidateCmd(t *testing.T) {
	cmd := "validate"
	testCases := []struct {
		args []string
	}{
		{[]string{cmd, t.TempDir()}},
		{[]string{cmd, t.TempDir(), "--connection", "sqlite_conn"}},
		{[]string{cmd, t.TempDir(), "--env", "dev"}},
		{[]string{cmd, t.TempDir(), "--verbose"}},
		{[]string{cmd, t.TempDir(), "--no-verbose"}},
	}
	for _, tc := range testCases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			err := execFlowCmd("init", tc.args[1])
			assert.NoError(t, err)

			err = execFlowCmd(tc.args...)
			assert.NoError(t, err)
		})
	}
}

func TestE2EFlowGenerateCmd(t *testing.T) {
	cmd := "generate"
	testCases := []struct {
		args []string
	}{
		{[]string{cmd, "example_basic_transform", "--project-dir", t.TempDir(), "--generate-tasks"}},
		{[]string{cmd, "example_templating", "--project-dir", t.TempDir(), "--no-generate-tasks"}},
		{[]string{cmd, "example_basic_transform", "--project-dir", t.TempDir(), "--no-verbose"}},
		{[]string{cmd, "example_templating", "--project-dir", t.TempDir(), "--verbose"}},
		{[]string{cmd, "example_basic_transform", "--project-dir", t.TempDir(), "--env", "default"}},
		{[]string{cmd, "example_templating", "--project-dir", t.TempDir(), "--env", "dev"}},
	}
	for _, tc := range testCases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			err := execFlowCmd("init", tc.args[3])
			assert.NoError(t, err)

			err = execFlowCmd(tc.args...)
			assert.NoError(t, err)
		})
	}
}

func TestE2EFlowRunCmd(t *testing.T) {
	cmd := "run"
	testCases := []struct {
		args []string
	}{
		{[]string{cmd, "example_basic_transform", "--project-dir", t.TempDir(), "--generate-tasks"}},
		{[]string{cmd, "example_templating", "--project-dir", t.TempDir(), "--no-generate-tasks"}},
		{[]string{cmd, "example_basic_transform", "--project-dir", t.TempDir(), "--no-verbose"}},
		{[]string{cmd, "example_templating", "--project-dir", t.TempDir(), "--verbose"}},
		{[]string{cmd, "example_basic_transform", "--project-dir", t.TempDir(), "--env", "default"}},
		{[]string{cmd, "example_templating", "--project-dir", t.TempDir(), "--env", "dev"}},
	}
	for _, tc := range testCases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			err := execFlowCmd("init", tc.args[3])
			assert.NoError(t, err)

			err = execFlowCmd(tc.args...)
			assert.NoError(t, err)
		})
	}
}

func TestE2EFlowRunCmdInUserHomeDir(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(homeDir, fmt.Sprintf("astro-cli-%s", t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	err = execFlowCmd("init", dir)
	assert.NoError(t, err)

	err = execFlowCmd("run", "example_basic_transform", "--project-dir", dir)
	assert.NoError(t, err)
}
