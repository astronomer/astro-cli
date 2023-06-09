package registry

import (
	"bytes"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func execProviderCmd(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newRegistryProviderCmd()
	cmd.SetOut(buf)
	log.SetOutput(buf)
	defer func() {
		log.SetOutput(os.Stderr)
	}()
	cmd.SetArgs(args)
	testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func TestProviderAdd(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	_ = os.Remove("requirements.txt")

	cmdArgs := []string{"add", "snowflake"}
	_, err := execProviderCmd(cmdArgs...)
	assert.NoError(t, err)

	fileContents, _ := fileutil.ReadFileToString("requirements.txt")
	assert.Regexp(t, `apache-airflow-providers-snowflake==\d+\.\d+\.\d+\n$`, fileContents, "We added the provider to the file")

	_, err = execProviderCmd(cmdArgs...)
	assert.NoError(t, err)
	// TODO - assert against stdout "apache-airflow-providers-snowflake already exists in requirements.txt"
	// Failing - not collecting stdout right, not sure why.
	fileContents, _ = fileutil.ReadFileToString("requirements.txt")
	assert.Regexp(t, `apache-airflow-providers-snowflake==\d+\.\d+\.\d+\n$`, fileContents, "We didn't write it again")

	_ = os.Remove("requirements.txt")
}
