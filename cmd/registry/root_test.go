package registry

import (
	"bytes"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRegistryCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	buf := new(bytes.Buffer)
	deplyCmd := newRegistryCmd()
	deplyCmd.SetOut(buf)
	testUtil.SetupOSArgsForGinkgo()
	_, err := deplyCmd.ExecuteC()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Astronomer Registry")
}

//func main() {
//	// DEMO
//	_ = os.Remove("requirements.txt")
//	_ = os.Remove("dags/sagemaker-batch-inference.py")
//
//	log.Info(ansi.Blue("DEMO: ADDING SPECIFIC PROVIDER BY NAME AND ID"))
//	const providerId = "apache-airflow-providers-airbyte"
//	const providerVersion = "3.2.0"
//	addProviderByIdAndVersion(providerId, providerVersion)
//
//	log.Info(ansi.Blue("DEMO: ADDING PROVIDER BY NAME"))
//	const providerName = "amazon"
//	addProviderByName(providerName)
//
//	log.Info(ansi.Blue("DEMO: ADDING DAG BY NAME AND VERSION"))
//	const dagName = "sagemaker-batch-inference"
//	const dagVersion = "1.0.1"
//	downloadDag(dagName, dagVersion, true)
//}
