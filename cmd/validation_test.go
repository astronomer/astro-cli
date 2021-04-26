package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateInvalidRole(t *testing.T) {
	err := validateRole("role")
	if err != nil && err.Error() != "please use one of: admin, editor, viewer" {
		t.Errorf("%s", err)
	}
}

func TestValidateValidRole(t *testing.T) {
	err := validateRole("admin")
	if err != nil {
		t.Errorf("%s", err)
	}
}

func TestValidateDagDeploymentArgs(t *testing.T) {
	myTests := []struct {
		dagDeploymentType, nfsLocation string
		expectedOutput                 string
		expectedError                  error
	}{
		{dagDeploymentType: "volume", nfsLocation: "test:/test", expectedError: nil},
		{dagDeploymentType: "image", expectedError: nil},
	}

	for _, tt := range myTests {
		actualError := validateDagDeploymentArgs(tt.dagDeploymentType, tt.nfsLocation)
		assert.NoError(t, actualError, "optional message here")
	}
}

func TestValidateDagDeploymentArgsErrors(t *testing.T) {
	myTests := []struct {
		dagDeploymentType, nfsLocation string
		expectedOutput                 string
		expectedError                  string
	}{
		{dagDeploymentType: "volume", expectedError: "please specify the nfs location via --nfs-location flag"},
		{dagDeploymentType: "unknown", expectedError: "please specify the correct DAG deployment type, one of the following: image, volume"},
	}

	for _, tt := range myTests {
		actualError := validateDagDeploymentArgs(tt.dagDeploymentType, tt.nfsLocation)
		assert.EqualError(t, actualError, tt.expectedError, "optional message here")
	}
}
