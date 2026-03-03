package registry

import (
	"bytes"
	"os"

	"github.com/astronomer/astro-cli/pkg/logger"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func execProviderCmd(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newRegistryProviderCmd(os.Stdout)
	cmd.SetOut(buf)
	logger.SetOutput(buf)
	defer func() {
		logger.SetOutput(os.Stderr)
	}()
	cmd.SetArgs(args)
	testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func (s *Suite) TestProviderAdd() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	server := newMockRegistryServer()
	defer server.Close()
	cleanup := setMockRegistry(server)
	defer cleanup()

	defer os.Remove("requirements.txt")

	cmdArgs := []string{"add", "snowflake"}
	resp, err := execProviderCmd(cmdArgs...)
	s.NoError(err)
	s.NotContains(resp, "I don't know why this is empty", "I don't know why stdout is empty")

	fileContents, _ := os.ReadFile("requirements.txt")
	s.Regexp(`apache-airflow-providers-snowflake==\d+\.\d+\.\d+\n$`, string(fileContents), "We added the provider to the file")

	_, err = execProviderCmd(cmdArgs...)
	s.NoError(err)
	fileContents, _ = os.ReadFile("requirements.txt")
	s.Regexp(`apache-airflow-providers-snowflake==\d+\.\d+\.\d+\n$`, string(fileContents), "We didn't write it again")

	_ = os.Remove("requirements.txt")
}
