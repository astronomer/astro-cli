package airflow

import (
	"testing"

	"github.com/astronomer/astro-cli/houston"
)

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
