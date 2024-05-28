package software

import (
	"github.com/astronomer/astro-cli/houston"

	"github.com/stretchr/testify/assert"
)

func (s *Suite) TestValidateDagDeploymentArgs() {
	myTests := []struct {
		dagDeploymentType, nfsLocation, gitRepoURL string
		acceptEmptyArgs                            bool
		expectedOutput                             string
		expectedError                              error
	}{
		{dagDeploymentType: houston.VolumeDeploymentType, nfsLocation: "test:/test", expectedError: nil},
		{dagDeploymentType: houston.ImageDeploymentType, expectedError: nil},
		{dagDeploymentType: houston.DagOnlyDeploymentType, expectedError: nil},
		{dagDeploymentType: houston.GitSyncDeploymentType, gitRepoURL: "https://github.com/neel-astro/private-airflow-dags-test", expectedError: nil},
		{dagDeploymentType: houston.GitSyncDeploymentType, gitRepoURL: "http://github.com/neel-astro/private-airflow-dags-test", expectedError: nil},
		{dagDeploymentType: houston.GitSyncDeploymentType, gitRepoURL: "git@github.com:neel-astro/private-airflow-dags-test.git", expectedError: nil},
		{dagDeploymentType: houston.GitSyncDeploymentType, gitRepoURL: "ssh://login@server.com:8080/~/private-airflow-dags-test.git", expectedError: nil},
		{dagDeploymentType: houston.GitSyncDeploymentType, gitRepoURL: "user@server.com:path/to/repo.git", expectedError: nil},
		{dagDeploymentType: houston.GitSyncDeploymentType, gitRepoURL: "git://server.com/~user/path/to/repo.git/", expectedError: nil},
		{dagDeploymentType: houston.GitSyncDeploymentType, gitRepoURL: "", acceptEmptyArgs: true, expectedError: nil},
	}

	for _, tt := range myTests {
		actualError := validateDagDeploymentArgs(tt.dagDeploymentType, tt.nfsLocation, tt.gitRepoURL, tt.acceptEmptyArgs)
		s.NoError(actualError, "optional message here")
	}
}

func (s *Suite) TestValidateDagDeploymentArgsErrors() {
	myTests := []struct {
		dagDeploymentType, nfsLocation, gitRepoURL string
		acceptEmptyArgs                            bool
		expectedOutput                             string
		expectedError                              string
	}{
		{dagDeploymentType: houston.VolumeDeploymentType, expectedError: "please specify the nfs location via --nfs-location flag"},
		{dagDeploymentType: "unknown", expectedError: ErrInvalidDAGDeploymentType.Error()},
		{dagDeploymentType: houston.ImageDeploymentType, expectedError: ""},
		{dagDeploymentType: houston.GitSyncDeploymentType, expectedError: "please specify a valid git repository URL via --git-repository-url"},
		{dagDeploymentType: houston.GitSyncDeploymentType, gitRepoURL: "/tmp/test/local-repo.git", expectedError: "please specify a valid git repository URL via --git-repository-url"},
		{dagDeploymentType: houston.GitSyncDeploymentType, gitRepoURL: "http://192.168.0.%31:8080/", expectedError: "please specify a valid git repository URL via --git-repository-url"},
	}

	for _, tt := range myTests {
		actualError := validateDagDeploymentArgs(tt.dagDeploymentType, tt.nfsLocation, tt.gitRepoURL, tt.acceptEmptyArgs)
		if tt.expectedError != "" {
			s.EqualError(actualError, tt.expectedError, "optional message here")
		} else {
			s.NoError(actualError)
		}
	}
}

func (s *Suite) Test_validateWorkspaceRole() {
	type args struct {
		role string
	}
	tests := []struct {
		name         string
		args         args
		errAssertion assert.ErrorAssertionFunc
	}{
		{
			name: "basic valid case",
			args: args{
				role: houston.WorkspaceAdminRole,
			},
			errAssertion: assert.NoError,
		},
		{
			name: "basic invalid case",
			args: args{
				role: "ADMIN",
			},
			errAssertion: assert.Error,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			tt.errAssertion(s.T(), validateWorkspaceRole(tt.args.role))
		})
	}
}

func (s *Suite) Test_validateDeploymentRole() {
	type args struct {
		role string
	}
	tests := []struct {
		name         string
		args         args
		errAssertion assert.ErrorAssertionFunc
	}{
		{
			name: "basic valid case",
			args: args{
				role: houston.DeploymentAdminRole,
			},
			errAssertion: assert.NoError,
		},
		{
			name: "basic invalid case",
			args: args{
				role: "ADMIN",
			},
			errAssertion: assert.Error,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			tt.errAssertion(s.T(), validateDeploymentRole(tt.args.role))
		})
	}
}

func (s *Suite) Test_ErrParsingKV() {
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
		s.Run(tt.name, func() {
			err := ErrParsingKV{kv: tt.args.kv}
			s.Error(err)
			s.Equal(err.Error(), tt.result)
		})
	}
}

func (s *Suite) Test_ErrInvalidArg() {
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
		s.Run(tt.name, func() {
			err := ErrInvalidArg{key: tt.args.key}
			s.Error(err)
			s.Equal(err.Error(), tt.result)
		})
	}
}

func (s *Suite) TestValidateExecutorArg() {
	type args struct {
		executor string
	}
	tests := []struct {
		name        string
		args        args
		result      string
		expectedErr string
	}{
		{
			name:        "valid local executor",
			args:        args{executor: localExecutorArg},
			result:      houston.LocalExecutorType,
			expectedErr: "",
		},
		{
			name:        "valid kubernetes executor",
			args:        args{executor: kubernetesExecutorArg},
			result:      houston.KubernetesExecutorType,
			expectedErr: "",
		},
		{
			name:        "valid kubernetes executor",
			args:        args{executor: k8sExecutorArg},
			result:      houston.KubernetesExecutorType,
			expectedErr: "",
		},
		{
			name:        "valid celery executor",
			args:        args{executor: celeryExecutorArg},
			result:      houston.CeleryExecutorType,
			expectedErr: "",
		},
		{
			name:        "invalid executor",
			args:        args{executor: "invalid test"},
			result:      "",
			expectedErr: "please specify correct executor, one of: local, celery, kubernetes, k8s",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			executorType, err := validateExecutorArg(tt.args.executor)
			s.Equal(tt.result, executorType)
			if tt.expectedErr != "" {
				s.EqualError(err, tt.expectedErr)
			}
		})
	}
}
