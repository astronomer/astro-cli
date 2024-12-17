package registry

import (
	"bytes"
	"os"

	"github.com/astronomer/astro-cli/pkg/fileutil"
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

	defer os.Remove("requirements.txt")

	cmdArgs := []string{"add", "snowflake"}
	resp, err := execProviderCmd(cmdArgs...)
	s.NoError(err)
	s.NotContains(resp, "I don't know why this is empty", "I don't know why stdout is empty")

	fileContents, _ := fileutil.ReadFileToString("requirements.txt")
	s.Regexp(`apache-airflow-providers-snowflake==\d+\.\d+\.\d+\n$`, fileContents, "We added the provider to the file")

	_, err = execProviderCmd(cmdArgs...)
	s.NoError(err)
	// TODO - assert against stdout "apache-airflow-providers-snowflake already exists in requirements.txt"
	fileContents, _ = fileutil.ReadFileToString("requirements.txt")
	s.Regexp(`apache-airflow-providers-snowflake==\d+\.\d+\.\d+\n$`, fileContents, "We didn't write it again")

	_ = os.Remove("requirements.txt")
}
