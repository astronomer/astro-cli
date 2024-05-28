package registry

import (
	"bytes"
	"os"

	"github.com/astronomer/astro-cli/pkg/fileutil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	log "github.com/sirupsen/logrus"
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

func (s *Suite) TestDagAdd() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	defer os.Remove("dags/sagemaker-batch-inference.py")

	cmdArgs := []string{"add", "sagemaker-batch-inference"}
	resp, err := execDagCmd(cmdArgs...)
	s.NoError(err)
	s.Contains(resp, "dags/sagemaker-batch-inference.py", "we mention where we downloaded it in stdout")

	fileContents, _ := fileutil.ReadFileToString("dags/sagemaker-batch-inference.py")
	s.Contains(fileContents, "airflow", "The DAG has words in it")
	s.Contains(fileContents, "DAG", "The DAG has words in it")

	_ = os.Remove("dags/sagemaker-batch-inference.py")
}
