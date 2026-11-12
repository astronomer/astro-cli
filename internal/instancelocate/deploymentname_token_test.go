package instancelocate

import (
	"strings"
	"testing"
)

// Under an API token a Deployment name has no workspace to be looked up in, and
// astro workspace switch is no way to give it one.
func TestDeploymentNameWithNoWorkspaceUnderATokenDoesNotAdviseASwitch(t *testing.T) {
	t.Setenv("ASTRO_API_TOKEN", "token")
	msg := (&DeploymentNameError{Name: "prod"}).Error()
	if !strings.Contains(msg, "ASTRO_API_TOKEN") || strings.Contains(msg, "workspace switch") {
		t.Errorf("message = %q", msg)
	}

	t.Setenv("ASTRO_API_TOKEN", "")
	if msg := (&DeploymentNameError{Name: "prod"}).Error(); !strings.Contains(msg, "astro workspace switch") {
		t.Errorf("a saved login's message = %q", msg)
	}
}
