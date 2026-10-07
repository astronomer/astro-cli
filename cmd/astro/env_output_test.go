package astro

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/env"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// mockOneVar answers the list call every variable read makes.
func mockOneVar(t *testing.T) *astrov1_mocks.ClientWithResponsesInterface {
	t.Helper()
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			{ObjectKey: "FOO", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "bar"}},
		}, TotalCount: 1},
	}, nil).Once()
	return mc
}

// requireNothingWritten fails if the command left a file in the working
// directory: --output used to name one, and must not any more.
func requireNothingWritten(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "the command wrote a file")
}

// -o is the format, as it is everywhere else in the CLI. Before v2,
// `--output json` here wrote the table to a file named json.
func TestEnvOutputIsTheFormat(t *testing.T) {
	for _, flag := range []string{"-o", "--output"} {
		t.Run(flag, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			defer resetEnvFlags()
			dir := t.TempDir()
			t.Chdir(dir)

			mc := mockOneVar(t)
			astroV1Client = mc

			out, err := execEnvCmd("variable", "list", "--workspace-id", "ws-test", flag, "json")
			require.NoError(t, err)
			var got struct {
				Variables []map[string]any `json:"variables"`
			}
			require.NoError(t, json.Unmarshal([]byte(out), &got), out)
			require.Len(t, got.Variables, 1)
			assert.Equal(t, "FOO", got.Variables[0]["object_key"])
			requireNothingWritten(t, dir)
			mc.AssertExpectations(t)
		})
	}
}

func TestEnvVarListDotenv(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := mockOneVar(t)
	astroV1Client = mc

	out, err := execEnvCmd("variable", "list", "--workspace-id", "ws-test", "-o", "dotenv")
	require.NoError(t, err)
	assert.Equal(t, "FOO=bar\n", out)
	mc.AssertExpectations(t)
}

// --output named a file before v2. A path is now just an unknown format:
// refused before anything is fetched, and nothing is written to it.
func TestEnvOutputRefusesAPath(t *testing.T) {
	for _, path := range []string{"vars.env", "out/vars", "-"} {
		t.Run(path, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			defer resetEnvFlags()
			dir := t.TempDir()
			t.Chdir(dir)

			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			astroV1Client = mc

			_, err := execEnvCmd("variable", "list", "--workspace-id", "ws-test", "--output", path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unknown output format")
			requireNothingWritten(t, dir)
			mc.AssertExpectations(t)
		})
	}
}

// The formats v2 dropped fail with the supported list, and dotenv is refused
// where there is no KEY=VALUE to write.
// The cross-kind listing has no values for dotenv to write. That is a value
// the command does not take, so a usage error, refused before any fetch.
func TestEnvListRefusesDotenvAsUsage(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mc

	_, err := execEnvCmd("list", "--workspace-id", "ws-test", "-o", "dotenv")
	require.ErrorIs(t, err, env.ErrInventoryHasNoValues)
	assert.True(t, cliout.IsUsage(err), "%v is not a usage error", err)
	mc.AssertExpectations(t)
}

func TestEnvOutputRefusesDroppedFormats(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"variable", "list", "-o", "yaml"}, `unknown output format "yaml" (supported: text, json, dotenv)`},
		{[]string{"variable", "get", "FOO", "-o", "yaml"}, `unknown output format "yaml" (supported: text, json, dotenv)`},
		{[]string{"connection", "list", "-o", "dotenv"}, `unknown output format "dotenv" (supported: text, json)`},
		{[]string{"airflow-variable", "get", "k", "-o", "dotenv"}, `unknown output format "dotenv" (supported: text, json)`},
		{[]string{"metrics-export", "list", "-o", "table"}, `unknown output format "table" (supported: text, json)`},
		{[]string{"connection", "link", "list", "--connection-key", "db", "-o", "yaml"}, `unknown output format "yaml" (supported: text, json)`},
		{[]string{"variable", "link", "list", "--variable-key", "K", "-o", "yaml"}, `unknown output format "yaml" (supported: text, json)`},
	}
	for _, c := range cases {
		t.Run(c.args[0]+" "+c.args[1], func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			defer resetEnvFlags()
			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			astroV1Client = mc

			_, err := execEnvCmd(append(c.args, "--workspace-id", "ws-test")...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.want)
			// A bad --output is a usage error here as everywhere: exit 2.
			assert.True(t, cliout.IsUsage(err), "%v is not a usage error", err)
			mc.AssertExpectations(t)
		})
	}
}

// The flags v2 took away fail as unknown flags, before anything is fetched.
// The old-to-new mapping lives in the release notes, not in the CLI.
func TestEnvRemovedFlagsFail(t *testing.T) {
	cases := [][]string{
		{"variable", "list", "--format", "json"},
		{"connection", "get", "db", "--format=json"},
		{"list", "--format", "json"},
		{"variable", "export", "--format", "dotenv"},
		{"variable", "export", "--output", ".env"},
		{"variable", "export", "-o", ".env"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			defer resetEnvFlags()
			dir := t.TempDir()
			t.Chdir(dir)
			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			astroV1Client = mc

			_, err := execEnvCmd(append(args, "--workspace-id", "ws-test")...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unknown")
			requireNothingWritten(t, dir)
			mc.AssertExpectations(t)
		})
	}
}

// Every `astro env` command with an --output has it as the format: -o, text
// by default, and no --format beside it. dotenv is offered by exactly the three
// variable reads: list, get and export.
func TestEnvOutputFlagIsUniform(t *testing.T) {
	dotenv := map[string]bool{
		"astro env variable list":   true,
		"astro env variable get":    true,
		"astro env variable export": true,
	}
	root := &cobra.Command{Use: "astro"}
	root.AddCommand(newEnvRootCmd(io.Discard))

	var walked, seen int
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			walk(sub)
		}
		walked++
		// c.Flag, not c.Flags().Lookup: cliout registers --output as a
		// persistent flag, which Flags() holds only once cobra has merged it.
		assert.Nil(t, c.Flag("format"), "%s still has --format", c.CommandPath())
		o := c.Flag("output")
		if o == nil {
			return
		}
		seen++
		assert.Equal(t, "o", o.Shorthand, c.CommandPath())
		assert.Equal(t, "text", o.DefValue, c.CommandPath())
		if dotenv[c.CommandPath()] {
			assert.Equal(t, "Output format: text, json or dotenv", o.Usage, c.CommandPath())
		} else {
			assert.Equal(t, "Output format: text or json", o.Usage, c.CommandPath())
		}
	}
	walk(root.Commands()[0])
	// The group declares it for every command under it, the groups and the
	// tombstones included, so a failure anywhere honors it.
	assert.Equal(t, walked, seen, "every astro env command reaches -o")
	assert.Greater(t, seen, 40)
}
