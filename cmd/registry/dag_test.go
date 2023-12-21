package registry

import (
	"bytes"
	"os"
	"testing"

	"github.com/astronomer/astro-cli/pkg/fileutil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func execDagCmd(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newRegistryDagCmd(os.Stdout)
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

func TestDagAdd(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	defer os.Remove("dags/sagemaker-batch-inference.py")

	cmdArgs := []string{"add", "sagemaker-batch-inference"}
	resp, err := execDagCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "dags/sagemaker-batch-inference.py", "we mention where we downloaded it in stdout")

	fileContents, _ := fileutil.ReadFileToString("dags/sagemaker-batch-inference.py")
	assert.Contains(t, fileContents, "airflow", "The DAG has words in it")
	assert.Contains(t, fileContents, "DAG", "The DAG has words in it")

	_ = os.Remove("dags/sagemaker-batch-inference.py")
}
