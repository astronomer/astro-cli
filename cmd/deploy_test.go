package cmd

import (
	"testing"

	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
)

func TestDeployRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand("deploy")
	assert.EqualError(t, err, "not in a project directory")
	assert.Contains(t, output, "astro deploy")
}

func TestDeploymentNameExists(t *testing.T) {
	deployments := []houston.Deployment{
		{ReleaseName: "dev"},
		{ReleaseName: "dev1"},
	}
	exists := deploymentNameExists("dev", deployments)
	if !exists {
		t.Errorf("deploymentNameExists(dev) = %t; want true", exists)
	}
}

func TestDeploymentNameDoesntExists(t *testing.T) {
	deployments := []houston.Deployment{
		{ReleaseName: "dummy"},
	}
	exists := deploymentNameExists("dev", deployments)
	if exists {
		t.Errorf("deploymentNameExists(dev) = %t; want false", exists)
	}
}

func Test_validImageRepo(t *testing.T) {
	assert.True(t, validImageRepo("quay.io/astronomer/ap-airflow"))
	assert.True(t, validImageRepo("astronomerinc/ap-airflow"))
	assert.False(t, validImageRepo("personal-repo/ap-airflow"))
}
