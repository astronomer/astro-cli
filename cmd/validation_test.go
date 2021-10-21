package cmd

import (
	"testing"

	"github.com/astronomer/astro-cli/houston"
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
		{dagDeploymentType: "image", expectedError: ""},
	}

	for _, tt := range myTests {
		actualError := validateDagDeploymentArgs(tt.dagDeploymentType, tt.nfsLocation)
		if tt.expectedError != "" {
			assert.EqualError(t, actualError, tt.expectedError, "optional message here")
		} else {
			assert.NoError(t, actualError)
		}
	}
}

func Test_validateWorkspaceRole(t *testing.T) {
	type args struct {
		role string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "basic valid case",
			args: args{
				role: "WORKSPACE_ADMIN",
			},
			wantErr: false,
		},
		{
			name: "basic invalid case",
			args: args{
				role: "ADMIN",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateWorkspaceRole(tt.args.role); (err != nil) != tt.wantErr {
				t.Errorf("validateWorkspaceRole() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_validateDeploymentRole(t *testing.T) {
	type args struct {
		role string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "basic valid case",
			args: args{
				role: houston.DeploymentAdmin,
			},
			wantErr: false,
		},
		{
			name: "basic invalid case",
			args: args{
				role: "ADMIN",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateDeploymentRole(tt.args.role); (err != nil) != tt.wantErr {
				t.Errorf("validateDeploymentRole() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_validateRole(t *testing.T) {
	type args struct {
		role string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "basic valid case",
			args: args{
				role: "admin",
			},
			wantErr: false,
		},
		{
			name: "basic invalid case",
			args: args{
				role: "ADMIN",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateRole(tt.args.role); (err != nil) != tt.wantErr {
				t.Errorf("validateRole() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_ErrParsingKV(t *testing.T) {
	type args struct {
		kv string
	}
	tests := []struct {
		name   string
		args   args
		result string
	}{
		{
			name:   "basic valid test",
			args:   args{kv: "test_key"},
			result: "failed to parse key value pair (test_key)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ErrParsingKV{kv: tt.args.kv}
			if err.Error() != tt.result {
				t.Errorf("ErrParsingKV invalid error string error = %v, wantErr %v", err.Error(), tt.result)
			}
		})
	}
}

func Test_ErrInvalidArg(t *testing.T) {
	type args struct {
		key string
	}
	tests := []struct {
		name   string
		args   args
		result string
	}{
		{
			name:   "basic valid test",
			args:   args{key: "test_key"},
			result: "invalid update arg key specified (test_key)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ErrInvalidArg{key: tt.args.key}
			if err.Error() != tt.result {
				t.Errorf("ErrParsingKV invalid error string error = %v, wantErr %v", err.Error(), tt.result)
			}
		})
	}
}
