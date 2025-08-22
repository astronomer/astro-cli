package software

import (
	"bytes"
	"errors"
	"os"
	"time"

	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/mock"
)

var (
	mockDeployment = &houston.Deployment{
		ID:                    "cknz133ra49758zr9w34b87ua",
		Type:                  "airflow",
		Label:                 "test",
		ReleaseName:           "accurate-radioactivity-8677",
		Version:               "0.15.6",
		AirflowVersion:        "2.0.0",
		DesiredAirflowVersion: "2.0.0",
		DeploymentInfo:        houston.DeploymentInfo{},
		Workspace: houston.Workspace{
			ID:    "ckn4phn1k0104v5xtrer5lpli",
			Label: "w1",
		},
		Urls: []houston.DeploymentURL{
			{URL: "https://deployments.local.astronomer.io/accurate-radioactivity-8677/airflow", Type: "airflow"},
			{URL: "https://deployments.local.astronomer.io/accurate-radioactivity-8677/flower", Type: "flower"},
		},
		CreatedAt: time.Time{},
		UpdatedAt: time.Time{},
	}
	mockDeploymentSA = &houston.ServiceAccount{
		ID:         "q1w2e3r4t5y6u7i8o9p0",
		APIKey:     "000000000000000000000000",
		Label:      "my_label",
		Category:   "default",
		LastUsedAt: "2019-10-16T21:14:22.105Z",
		CreatedAt:  "2019-10-16T21:14:22.105Z",
		UpdatedAt:  "2019-10-16T21:14:22.105Z",
		Active:     true,
	}
	mockDeploymentUserRole = &houston.RoleBinding{
		Role: houston.DeploymentViewerRole,
		User: houston.RoleBindingUser{
			Username: "somebody@astronomer.io",
		},
		Deployment: houston.Deployment{
			ID:          "ckggvxkw112212kc9ebv8vu6p",
			ReleaseName: "prehistoric-gravity-9229",
		},
	}
	mockAppConfig = &houston.AppConfig{
		Flags: houston.FeatureFlags{
			AstroRuntimeEnabled: true,
		},
	}
)

func execDeploymentCmd(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newDeploymentRootCmd(buf)
	cmd.SetOut(buf)
	cmd.SetArgs(args)
	testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func (s *Suite) TestDeploymentRootCommand() {
	output, err := execDeploymentCmd()
	s.NoError(err)
	s.Contains(output, "deployment [command]")
}

func (s *Suite) TestDeploymentCreateCommandNfsMountDisabled() {
	appConfig = &houston.AppConfig{
		NfsMountDagDeployment: false,
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(appConfig, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=celery", "--dag-deployment-type=volume", "--nfs-location=test:/test"}, expectedOutput: "", expectedError: "unknown flag: --dag-deployment-type"},
		// test default executor is celery if --executor flag is not provided
		{cmdArgs: []string{"create", "--label=new-deployment-name"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=celery"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
	}
	for _, tt := range myTests {
		houstonClient = api
		output, err := execDeploymentCmd(tt.cmdArgs...)
		if tt.expectedError != "" {
			s.EqualError(err, tt.expectedError)
		} else {
			s.NoError(err)
		}
		s.Contains(output, tt.expectedOutput)
	}
}

func (s *Suite) TestDeploymentCreateCommandTriggererDisabled() {
	appConfig = &houston.AppConfig{TriggererEnabled: false}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(appConfig, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=celery", "--triggerer-replicas=1"}, expectedOutput: "", expectedError: "unknown flag: --triggerer-replicas"},
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=celery"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
	}
	for _, tt := range myTests {
		houstonClient = api
		output, err := execDeploymentCmd(tt.cmdArgs...)
		if tt.expectedError != "" {
			s.EqualError(err, tt.expectedError)
		} else {
			s.NoError(err)
		}
		s.Contains(output, tt.expectedOutput)
	}
}

func (s *Suite) TestDeploymentCreateCommandTriggererEnabled() {
	appConfig = &houston.AppConfig{
		TriggererEnabled: true,
		Flags: houston.FeatureFlags{
			TriggererEnabled: true,
		},
	}
	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(appConfig, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=celery", "--triggerer-replicas=1"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
	}
	for _, tt := range myTests {
		houstonClient = api
		output, err := execDeploymentCmd(tt.cmdArgs...)
		if tt.expectedError != "" {
			s.EqualError(err, tt.expectedError)
		} else {
			s.NoError(err)
		}
		s.Contains(output, tt.expectedOutput)
	}
}

func (s *Suite) TestDeploymentCreateCommandNfsMountEnabled() {
	appConfig = &houston.AppConfig{
		NfsMountDagDeployment: true,
		Flags: houston.FeatureFlags{
			NfsMountDagDeployment: true,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(appConfig, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil).Times(2)

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=celery", "--dag-deployment-type=volume", "--nfs-location=test:/test"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=celery"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=celery", "--dag-deployment-type=dummy"}, expectedOutput: "", expectedError: ErrInvalidDAGDeploymentType.Error()},
	}
	for _, tt := range myTests {
		houstonClient = api
		output, err := execDeploymentCmd(tt.cmdArgs...)
		if tt.expectedError != "" {
			s.EqualError(err, tt.expectedError)
		} else {
			s.NoError(err)
		}
		s.Contains(output, tt.expectedOutput)
	}
}

func (s *Suite) TestDeploymentCreateCommandGitSyncEnabled() {
	appConfig = &houston.AppConfig{
		Flags: houston.FeatureFlags{
			GitSyncEnabled: true,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(appConfig, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil).Times(5)

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=celery", "--dag-deployment-type=git_sync", "--git-repository-url=https://github.com/bote795/public-ariflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--sync-interval=200"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=local", "--dag-deployment-type=git_sync", "-u=https://github.com/bote795/public-ariflow-dags-test.git", "-p=dagscopy/", "-b=main", "-s=200"}, expectedOutput: "Successfully created deployment with Local executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=celery", "--dag-deployment-type=git_sync", "--git-repository-url=git@github.com:bote795/private-ariflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--ssh-key=./testfiles/ssh_key", "--known-hosts=./testfiles/known_hosts"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=celery", "--dag-deployment-type=git_sync", "--git-repository-url=git@github.com:neel-astro/private-airflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--ssh-key=./testfiles/ssh_key", "--known-hosts=./testfiles/known_hosts"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=celery", "--dag-deployment-type=git_sync", "--git-repository-url=git@github.com:neel-astro/private-airflow-dags-test.git", "--ssh-key=./testfiles/ssh_key", "--known-hosts=./testfiles/known_hosts"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=celery", "--dag-deployment-type=dummy"}, expectedOutput: "", expectedError: ErrInvalidDAGDeploymentType.Error()},
	}
	for _, tt := range myTests {
		houstonClient = api
		output, err := execDeploymentCmd(tt.cmdArgs...)
		if tt.expectedError != "" {
			s.EqualError(err, tt.expectedError)
		} else {
			s.NoError(err)
		}
		s.Contains(output, tt.expectedOutput)
	}
}

func (s *Suite) TestDeploymentCreateCommandDagOnlyDeployEnabled() {
	appConfig = &houston.AppConfig{
		Flags: houston.FeatureFlags{
			DagOnlyDeployment: true,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(appConfig, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil).Times(5)

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=celery", "--dag-deployment-type=dag_deploy", "--force"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=celery", "--dag-deployment-type=dummy", "--force"}, expectedOutput: "", expectedError: ErrInvalidDAGDeploymentType.Error()},
	}
	for _, tt := range myTests {
		houstonClient = api
		output, err := execDeploymentCmd(tt.cmdArgs...)
		if tt.expectedError != "" {
			s.EqualError(err, tt.expectedError)
		} else {
			s.NoError(err)
		}
		s.Contains(output, tt.expectedOutput)
	}
}

func (s *Suite) TestDeploymentCreateCommandGitSyncDisabled() {
	appConfig = &houston.AppConfig{
		Flags: houston.FeatureFlags{GitSyncEnabled: false},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(appConfig, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=celery", "--dag-deployment-type=git_sync", "--git-repository-url=https://github.com/bote795/public-ariflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--sync-interval=200"}, expectedOutput: "", expectedError: "unknown flag: --dag-deployment-type"},
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=celery"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
	}
	for _, tt := range myTests {
		houstonClient = api
		output, err := execDeploymentCmd(tt.cmdArgs...)
		if tt.expectedError != "" {
			s.EqualError(err, tt.expectedError)
		} else {
			s.NoError(err)
		}
		s.Contains(output, tt.expectedOutput)
	}
}

func (s *Suite) TestDeploymentCreateCommandGitSyncDisabledAndVersionIs1_0_0AndClusterIDIsNotSet() {
	appConfig = &houston.AppConfig{
		Flags: houston.FeatureFlags{GitSyncEnabled: false},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(appConfig, nil)
	api.On("GetPlatformVersion", nil).Return("1.0.0", nil)
	api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=celery"}, expectedOutput: "", expectedError: "required flag(s) \"cluster-id\" not set"},
	}
	for _, tt := range myTests {
		houstonClient = api
		output, err := execDeploymentCmd(tt.cmdArgs...)
		if tt.expectedError != "" {
			s.EqualError(err, tt.expectedError)
		} else {
			s.NoError(err)
		}
		s.Contains(output, tt.expectedOutput)
	}
}

func (s *Suite) TestDeploymentCreateWithTypeDagDeploy() {
	s.Run("user should not be prompted if deployment type is not dag_deploy", func() {
		appConfig = &houston.AppConfig{
			TriggererEnabled: true,
			Flags: houston.FeatureFlags{
				TriggererEnabled: true,
			},
		}
		api := new(mocks.ClientInterface)
		api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

		cmdArgs := []string{"create", "--label=new-deployment-name", "--executor=celery", "--triggerer-replicas=1"}
		expectedOutput := "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs"
		houstonClient = api
		output, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(output, expectedOutput)
		api.AssertExpectations(s.T())
	})

	s.Run("user should not be prompted if deployment type is dag_deploy but -f flag is sent", func() {
		appConfig = &houston.AppConfig{
			TriggererEnabled: true,
			Flags: houston.FeatureFlags{
				TriggererEnabled:  true,
				DagOnlyDeployment: true,
			},
		}
		api := new(mocks.ClientInterface)
		api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

		cmdArgs := []string{"create", "--label=new-deployment-name", "--executor=celery", "--dag-deployment-type=dag_deploy", "--triggerer-replicas=1", "--force"}
		expectedOutput := "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs"
		houstonClient = api
		output, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(output, expectedOutput)
		api.AssertExpectations(s.T())
	})

	s.Run("user should be prompted if deployment type is dag_deploy and -f flag is not sent. No deployment is created without confirmation.", func() {
		appConfig = &houston.AppConfig{
			TriggererEnabled: true,
			Flags: houston.FeatureFlags{
				TriggererEnabled:  true,
				DagOnlyDeployment: true,
			},
		}

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", "").Return(appConfig, nil)
		api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)
		cmdArgs := []string{"create", "--label=new-deployment-name", "--executor=celery", "--dag-deployment-type=dag_deploy", "--triggerer-replicas=1"}
		houstonClient = api

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		defer testUtil.MockUserInput(s.T(), "n")()

		_, err = execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		// Houston CreateDeployment API should not be called if user does not confirm the prompt
		api.AssertNotCalled(s.T(), "CreateDeployment", mock.Anything)
	})

	s.Run("user should be prompted if deployment type is dag_deploy and -f flag is not sent. Deployment is created with confirmation.", func() {
		appConfig = &houston.AppConfig{
			TriggererEnabled: true,
			Flags: houston.FeatureFlags{
				TriggererEnabled:  true,
				DagOnlyDeployment: true,
			},
		}

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", "").Return(appConfig, nil)
		api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)
		cmdArgs := []string{"create", "--label=new-deployment-name", "--executor=celery", "--dag-deployment-type=dag_deploy", "--triggerer-replicas=1"}
		houstonClient = api

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		defer testUtil.MockUserInput(s.T(), "y")()

		_, err = execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		// Houston CreateDeployment API should be called if user confirms the prompt
		api.AssertCalled(s.T(), "CreateDeployment", mock.Anything)
	})
}

func (s *Suite) TestDeploymentUpdateWithTypeDagDeploy() {
	s.Run("user should not be prompted if new deployment type is not dag_deploy", func() {
		appConfig = &houston.AppConfig{
			Flags: houston.FeatureFlags{
				DagOnlyDeployment: true,
			},
		}

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", "").Return(appConfig, nil)
		api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
		api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil)
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.ImageDeploymentType,
		}
		deployment := &houston.Deployment{
			DagDeployment: *dagDeployment,
		}
		api.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()
		cmdArgs := []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--dag-deployment-type=image"}
		houstonClient = api
		output, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(output, "Successfully updated deployment")
	})

	s.Run("GetDeployment throws an error", func() {
		appConfig = &houston.AppConfig{
			Flags: houston.FeatureFlags{
				DagOnlyDeployment: true,
			},
		}

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", "").Return(appConfig, nil)
		api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil)
		api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
		getDeploymentError := errors.New("Test error")
		api.On("GetDeployment", mock.Anything).Return(nil, getDeploymentError).Once()
		cmdArgs := []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--dag-deployment-type=dag_deploy"}
		houstonClient = api
		_, err := execDeploymentCmd(cmdArgs...)
		s.EqualError(err, "failed to get deployment info: "+getDeploymentError.Error())
	})

	s.Run("user should not be prompted if new deployment type is dag_deploy and current deployment type is also dag_deploy", func() {
		appConfig = &houston.AppConfig{
			Flags: houston.FeatureFlags{
				DagOnlyDeployment: true,
			},
		}

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", "").Return(appConfig, nil)
		api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil)
		api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			DagDeployment: *dagDeployment,
		}
		api.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()
		cmdArgs := []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--dag-deployment-type=dag_deploy"}
		houstonClient = api
		output, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(output, "Successfully updated deployment")
	})

	s.Run("user should be prompted if new deployment type is dag_deploy and current deployment type is not dag_deploy. User rejects.", func() {
		appConfig = &houston.AppConfig{
			Flags: houston.FeatureFlags{
				DagOnlyDeployment: true,
			},
		}

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", "").Return(appConfig, nil)
		api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil)
		api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.ImageDeploymentType,
		}
		deployment := &houston.Deployment{
			DagDeployment: *dagDeployment,
		}
		api.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()
		cmdArgs := []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--dag-deployment-type=dag_deploy"}
		houstonClient = api

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		defer testUtil.MockUserInput(s.T(), "n")()

		_, err = execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		api.AssertCalled(s.T(), "GetDeployment", mock.Anything)
		// Houston UpdateDeployment API should not be called if user does not confirm the prompt
		api.AssertNotCalled(s.T(), "UpdateDeployment", mock.Anything)
	})

	s.Run("user should be prompted if new deployment type is dag_deploy and current deployment type is not dag_deploy. User confirms.", func() {
		appConfig = &houston.AppConfig{
			Flags: houston.FeatureFlags{
				DagOnlyDeployment: true,
			},
		}

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", "").Return(appConfig, nil)
		api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil)
		api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.ImageDeploymentType,
		}
		deployment := &houston.Deployment{
			DagDeployment: *dagDeployment,
		}
		api.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()
		cmdArgs := []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--dag-deployment-type=dag_deploy"}
		houstonClient = api

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		defer testUtil.MockUserInput(s.T(), "y")()

		_, err = execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		api.AssertCalled(s.T(), "GetDeployment", mock.Anything)
		// Houston UpdateDeployment API should be called if user confirms the prompt
		api.AssertCalled(s.T(), "UpdateDeployment", mock.Anything)
	})
}

func (s *Suite) TestDeploymentUpdateFromTypeDagDeployToNonDagDeploy() {
	s.Run("user should be prompted if new deployment type is not dag_deploy but current type is dag_deploy. User rejects.", func() {
		appConfig = &houston.AppConfig{
			Flags: houston.FeatureFlags{
				DagOnlyDeployment: true,
			},
		}
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", "").Return(appConfig, nil)
		api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil)
		api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			DagDeployment: *dagDeployment,
		}
		api.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()
		cmdArgs := []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--dag-deployment-type=image"}
		houstonClient = api

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		defer testUtil.MockUserInput(s.T(), "n")()

		_, err = execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		api.AssertCalled(s.T(), "GetDeployment", mock.Anything)
		// Houston UpdateDeployment API should not be called if user does not confirm the prompt
		api.AssertNotCalled(s.T(), "UpdateDeployment", mock.Anything)
	})

	s.Run("user should be prompted if new deployment type is not dag_deploy but current type is dag_deploy. User confirms.", func() {
		appConfig = &houston.AppConfig{
			Flags: houston.FeatureFlags{
				DagOnlyDeployment: true,
			},
		}

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", "").Return(appConfig, nil)
		api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil)
		api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			DagDeployment: *dagDeployment,
		}
		api.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()
		cmdArgs := []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--dag-deployment-type=image"}
		houstonClient = api

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		defer testUtil.MockUserInput(s.T(), "y")()

		_, err = execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		api.AssertCalled(s.T(), "GetDeployment", mock.Anything)
		// Houston UpdateDeployment API should be called if user does not confirm the prompt
		api.AssertCalled(s.T(), "UpdateDeployment", mock.Anything)
	})
}

func (s *Suite) TestDeploymentUpdateCommand() {
	appConfig = &houston.AppConfig{
		NfsMountDagDeployment: true,
		Flags: houston.FeatureFlags{
			NfsMountDagDeployment: true,
			GitSyncEnabled:        true,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(appConfig, nil)
	api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil).Times(8)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	dagDeployment := &houston.DagDeploymentConfig{
		Type: houston.ImageDeploymentType,
	}
	deployment := &houston.Deployment{
		DagDeployment: *dagDeployment,
	}
	api.On("GetDeployment", mock.Anything).Return(deployment, nil).Times(9)

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--dag-deployment-type=volume", "--nfs-location=test:/test"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--dag-deployment-type=git_sync", "--git-repository-url=https://github.com/bote795/public-ariflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--sync-interval=200"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--dag-deployment-type=git_sync", "-u=https://github.com/bote795/public-ariflow-dags-test.git", "-p=dagscopy/", "-b=main", "-s=200"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--dag-deployment-type=git_sync", "--git-repository-url=git@github.com:bote795/private-ariflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--ssh-key=./testfiles/ssh_key", "--known-hosts=./testfiles/known_hosts"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--dag-deployment-type=git_sync", "--git-repository-url=git@github.com:neel-astro/private-airflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--ssh-key=./testfiles/ssh_key", "--known-hosts=./testfiles/known_hosts"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--dag-deployment-type=git_sync", "--git-repository-url=git@github.com:neel-astro/private-airflow-dags-test.git", "--ssh-key=./testfiles/ssh_key", "--known-hosts=./testfiles/known_hosts"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--dag-deployment-type=wrong", "--nfs-location=test:/test"}, expectedOutput: "", expectedError: ErrInvalidDAGDeploymentType.Error()},
		{cmdArgs: []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--executor=local"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"update", "cknrml96n02523xr97ygj95n5", "--cloud-role=arn:aws:iam::1234567890:role/test_role4c2301381e"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
	}
	for _, tt := range myTests {
		houstonClient = api
		output, err := execDeploymentCmd(tt.cmdArgs...)
		if tt.expectedError != "" {
			s.EqualError(err, tt.expectedError)
		} else {
			s.NoError(err)
		}
		s.Contains(output, tt.expectedOutput)
	}
}

func (s *Suite) TestDeploymentUpdateTriggererEnabledCommand() {
	appConfig = &houston.AppConfig{
		TriggererEnabled: true,
		Flags:            houston.FeatureFlags{TriggererEnabled: true},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(appConfig, nil)
	api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil).Twice()
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	dagDeployment := &houston.DagDeploymentConfig{
		Type: houston.ImageDeploymentType,
	}
	deployment := &houston.Deployment{
		DagDeployment: *dagDeployment,
	}
	api.On("GetDeployment", mock.Anything).Return(deployment, nil).Times(2)
	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--triggerer-replicas=1"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
	}
	for _, tt := range myTests {
		houstonClient = api
		output, err := execDeploymentCmd(tt.cmdArgs...)
		if tt.expectedError != "" {
			s.EqualError(err, tt.expectedError)
		} else {
			s.NoError(err)
		}
		s.Contains(output, tt.expectedOutput)
	}
}

func (s *Suite) TestDeploymentUpdateCommandGitSyncDisabled() {
	appConfig = &houston.AppConfig{
		NfsMountDagDeployment: true,
		Flags: houston.FeatureFlags{
			NfsMountDagDeployment: true,
			GitSyncEnabled:        false,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(appConfig, nil)
	api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	dagDeployment := &houston.DagDeploymentConfig{
		Type: houston.ImageDeploymentType,
	}
	deployment := &houston.Deployment{
		DagDeployment: *dagDeployment,
	}
	api.On("GetDeployment", mock.Anything).Return(deployment, nil).Times(2)
	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--dag-deployment-type=volume", "--nfs-location=test:/test"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--dag-deployment-type=git_sync", "--git-repository-url=https://github.com/bote795/public-ariflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--sync-interval=200"}, expectedOutput: "", expectedError: "unknown flag: --git-repository-url"},
	}
	for _, tt := range myTests {
		houstonClient = api
		output, err := execDeploymentCmd(tt.cmdArgs...)
		if tt.expectedError != "" {
			s.EqualError(err, tt.expectedError)
		} else {
			s.NoError(err)
		}
		s.Contains(output, tt.expectedOutput)
	}
}

func (s *Suite) TestDeploymentUpdateCommandDagOnlyDeployEnabled() {
	appConfig = &houston.AppConfig{
		Flags: houston.FeatureFlags{
			DagOnlyDeployment: true,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(appConfig, nil)
	api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	dagDeployment := &houston.DagDeploymentConfig{
		Type: houston.ImageDeploymentType,
	}
	deployment := &houston.Deployment{
		DagDeployment: *dagDeployment,
	}
	api.On("GetDeployment", mock.Anything).Return(deployment, nil)
	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--dag-deployment-type=dag_deploy", "--force"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--dag-deployment-type=invalid", "--force"}, expectedOutput: "", expectedError: ErrInvalidDAGDeploymentType.Error()},
	}
	for _, tt := range myTests {
		houstonClient = api
		output, err := execDeploymentCmd(tt.cmdArgs...)
		if tt.expectedError != "" {
			s.EqualError(err, tt.expectedError)
		} else {
			s.NoError(err)
		}
		s.Contains(output, tt.expectedOutput)
	}
}

func (s *Suite) TestDeploymentAirflowUpgradeCommand() {
	expectedOut := `The upgrade from Airflow 1.10.5 to 1.10.10 has been started. To complete this process, add an Airflow 1.10.10 image to your Dockerfile and deploy to Astronomer.`

	mockDeploymentResponse := *mockDeployment
	mockDeploymentResponse.AirflowVersion = "1.10.5"
	mockDeploymentResponse.DesiredAirflowVersion = "1.10.10"

	mockUpdateRequest := map[string]interface{}{
		"deploymentId":          mockDeploymentResponse.ID,
		"desiredAirflowVersion": mockDeploymentResponse.DesiredAirflowVersion,
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(mockAppConfig, nil)
	api.On("GetDeployment", mockDeploymentResponse.ID).Return(&mockDeploymentResponse, nil)
	api.On("UpdateDeploymentAirflow", mockUpdateRequest).Return(&mockDeploymentResponse, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	houstonClient = api
	output, err := execDeploymentCmd(
		"airflow",
		"upgrade",
		"--deployment-id="+mockDeploymentResponse.ID,
		"--desired-airflow-version="+mockDeploymentResponse.DesiredAirflowVersion,
	)
	s.NoError(err)
	s.Contains(output, expectedOut)
}

func (s *Suite) TestDeploymentAirflowUpgradeCancelCommand() {
	expectedOut := `Airflow upgrade process has been successfully canceled. Your Deployment was not interrupted and you are still running Airflow 1.10.5.`

	mockDeploymentResponse := *mockDeployment
	mockDeploymentResponse.AirflowVersion = "1.10.5"
	mockDeploymentResponse.DesiredAirflowVersion = "1.10.10"

	expectedUpdateRequest := map[string]interface{}{
		"deploymentId":          mockDeploymentResponse.ID,
		"desiredAirflowVersion": mockDeploymentResponse.AirflowVersion,
	}

	mockDeploymentUpdated := mockDeploymentResponse
	mockDeploymentUpdated.DesiredAirflowVersion = mockDeploymentUpdated.AirflowVersion

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(mockAppConfig, nil)
	api.On("GetDeployment", mockDeploymentResponse.ID).Return(&mockDeploymentResponse, nil)
	api.On("UpdateDeploymentAirflow", expectedUpdateRequest).Return(&mockDeploymentUpdated, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	houstonClient = api
	output, err := execDeploymentCmd(
		"airflow",
		"upgrade",
		"--cancel",
		"--deployment-id="+mockDeploymentResponse.ID,
	)
	s.NoError(err)
	s.Contains(output, expectedOut)
}

func (s *Suite) TestDeploymentDelete() {
	expectedOut := `Successfully deleted deployment`

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(mockAppConfig, nil)
	api.On("DeleteDeployment", houston.DeleteDeploymentRequest{DeploymentID: mockDeployment.ID, HardDelete: false}).Return(mockDeployment, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	houstonClient = api
	output, err := execDeploymentCmd("delete", mockDeployment.ID)
	s.NoError(err)
	s.Contains(output, expectedOut)
}

func (s *Suite) TestDeploymentList() {
	expectedRequest := houston.PaginatedDeploymentsRequest{
		Take: -1,
	}

	api := new(mocks.ClientInterface)
	api.On("ListPaginatedDeployments", expectedRequest).Return([]houston.Deployment{*mockDeployment}, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	houstonClient = api
	output, err := execDeploymentCmd("list", "--all")
	s.NoError(err)
	s.Contains(output, mockDeployment.ID)
	api.AssertExpectations(s.T())
}

func (s *Suite) TestDeploymentListWithClusterID() {
	expectedRequest := houston.PaginatedDeploymentsRequest{
		Take:      -1,
		ClusterID: "testClusterID",
	}

	api := new(mocks.ClientInterface)
	api.On("ListPaginatedDeployments", expectedRequest).Return([]houston.Deployment{*mockDeployment}, nil)
	api.On("GetPlatformVersion", nil).Return("1.0.0", nil)
	houstonClient = api
	output, err := execDeploymentCmd("list", "--all", "--cluster-id=testClusterID")
	s.NoError(err)
	s.Contains(output, mockDeployment.ID)
	api.AssertExpectations(s.T())
}

func (s *Suite) TestDeploymentDeleteHardResponseNo() {
	appConfig = &houston.AppConfig{
		HardDeleteDeployment: true,
		Flags: houston.FeatureFlags{
			HardDeleteDeployment: true,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(appConfig, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	// mock os.Stdin
	input := []byte("n")
	r, w, err := os.Pipe()
	s.Require().NoError(err)
	_, err = w.Write(input)
	s.NoError(err)
	w.Close()
	stdin := os.Stdin
	// Restore stdin right after the test.
	defer func() { os.Stdin = stdin }()
	os.Stdin = r

	houstonClient = api
	_, err = execDeploymentCmd("delete", "--hard", mockDeployment.ID)
	s.NoError(err)
}

func (s *Suite) TestDeploymentDeleteHardResponseYes() {
	expectedOut := `Successfully deleted deployment`
	appConfig = &houston.AppConfig{
		HardDeleteDeployment: true,
		Flags: houston.FeatureFlags{
			HardDeleteDeployment: true,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(appConfig, nil)
	api.On("DeleteDeployment", houston.DeleteDeploymentRequest{DeploymentID: mockDeployment.ID, HardDelete: true}).Return(mockDeployment, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	// mock os.Stdin
	input := []byte("y")
	r, w, err := os.Pipe()
	s.Require().NoError(err)
	_, err = w.Write(input)
	s.NoError(err)
	w.Close()
	stdin := os.Stdin
	// Restore stdin right after the test.
	defer func() { os.Stdin = stdin }()
	os.Stdin = r

	houstonClient = api
	output, err := execDeploymentCmd("delete", "--hard", mockDeployment.ID)
	s.NoError(err)
	s.Contains(output, expectedOut)
}

func (s *Suite) TestDeploymentRuntimeUpgradeCommand() {
	appConfig = &houston.AppConfig{
		Flags: houston.FeatureFlags{
			AstroRuntimeEnabled: true,
		},
	}

	expectedOut := `The upgrade from Runtime 4.2.4 to 4.2.5 has been started. To complete this process, add an Runtime 4.2.5 image to your Dockerfile and deploy to Astronomer.`

	mockDeploymentResponse := *mockDeployment
	mockDeploymentResponse.AirflowVersion = ""
	mockDeploymentResponse.DesiredAirflowVersion = ""
	mockDeploymentResponse.RuntimeVersion = "4.2.4"
	mockDeploymentResponse.RuntimeAirflowVersion = "2.2.5"
	mockDeploymentResponse.DesiredRuntimeVersion = "4.2.5"

	mockUpdateRequest := map[string]interface{}{
		"deploymentUuid":        mockDeploymentResponse.ID,
		"desiredRuntimeVersion": mockDeploymentResponse.DesiredRuntimeVersion,
	}

	api := new(mocks.ClientInterface)
	api.On("GetDeployment", mockDeploymentResponse.ID).Return(&mockDeploymentResponse, nil)
	api.On("UpdateDeploymentRuntime", mockUpdateRequest).Return(&mockDeploymentResponse, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	houstonClient = api
	output, err := execDeploymentCmd(
		"runtime",
		"upgrade",
		"--deployment-id="+mockDeploymentResponse.ID,
		"--desired-runtime-version="+mockDeploymentResponse.DesiredRuntimeVersion,
	)
	s.NoError(err)
	s.Contains(output, expectedOut)
}

func (s *Suite) TestDeploymentRuntimeUpgradeCancelCommand() {
	appConfig = &houston.AppConfig{
		Flags: houston.FeatureFlags{
			AstroRuntimeEnabled: true,
		},
	}

	expectedOut := `Runtime upgrade process has been successfully canceled. Your Deployment was not interrupted and you are still running Runtime 4.2.4.`

	mockDeploymentResponse := *mockDeployment
	mockDeploymentResponse.AirflowVersion = ""
	mockDeploymentResponse.DesiredAirflowVersion = ""
	mockDeploymentResponse.RuntimeVersion = "4.2.4"
	mockDeploymentResponse.DesiredRuntimeVersion = "4.2.5"
	mockDeploymentResponse.RuntimeAirflowVersion = "2.2.5"

	expectedUpdateRequest := map[string]interface{}{
		"deploymentUuid": mockDeploymentResponse.ID,
	}

	mockDeploymentUpdated := mockDeploymentResponse
	mockDeploymentUpdated.DesiredRuntimeVersion = mockDeploymentUpdated.RuntimeVersion

	api := new(mocks.ClientInterface)
	api.On("GetDeployment", mockDeploymentResponse.ID).Return(&mockDeploymentResponse, nil)
	api.On("CancelUpdateDeploymentRuntime", expectedUpdateRequest).Return(&mockDeploymentUpdated, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	houstonClient = api
	output, err := execDeploymentCmd(
		"runtime",
		"upgrade",
		"--cancel",
		"--deployment-id="+mockDeploymentResponse.ID,
	)
	s.NoError(err)
	s.Contains(output, expectedOut)
	api.AssertExpectations(s.T())
}

func (s *Suite) TestDeploymentRuntimeMigrateCommand() {
	appConfig = &houston.AppConfig{
		Flags: houston.FeatureFlags{
			AstroRuntimeEnabled: true,
		},
	}

	expectedOut := `The migration from Airflow 2.2.4 image to Runtime 4.2.4 has been started. To complete this process, add an Runtime 4.2.4 image to your Dockerfile and deploy to Astronomer.`

	mockDeploymentResponse := *mockDeployment
	mockDeploymentResponse.AirflowVersion = "2.2.4"
	mockDeploymentResponse.DesiredAirflowVersion = "2.2.4"

	mockUpdateRequest := map[string]interface{}{
		"deploymentUuid":        mockDeploymentResponse.ID,
		"desiredRuntimeVersion": "4.2.4",
	}

	mockRuntimeReleaseResp := houston.RuntimeReleases{houston.RuntimeRelease{AirflowVersion: "2.2.4", Version: "4.2.4"}}

	api := new(mocks.ClientInterface)
	api.On("GetDeployment", mockDeploymentResponse.ID).Return(&mockDeploymentResponse, nil)
	api.On("UpdateDeploymentRuntime", mockUpdateRequest).Return(&mockDeploymentResponse, nil)
	vars := make(map[string]interface{})
	vars["airflowVersion"] = mockDeploymentResponse.AirflowVersion
	vars["clusterId"] = ""
	api.On("GetRuntimeReleases", vars).Return(mockRuntimeReleaseResp, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	houstonClient = api
	output, err := execDeploymentCmd(
		"runtime",
		"migrate",
		"--deployment-id="+mockDeploymentResponse.ID,
	)
	s.NoError(err)
	s.Contains(output, expectedOut)
	api.AssertExpectations(s.T())
}

func (s *Suite) TestDeploymentRuntimeMigrateCommandFor1_0_0() {
	appConfig = &houston.AppConfig{
		Flags: houston.FeatureFlags{
			AstroRuntimeEnabled: true,
		},
	}

	expectedOut := `The migration from Airflow 2.2.4 image to Runtime 4.2.4 has been started. To complete this process, add an Runtime 4.2.4 image to your Dockerfile and deploy to Astronomer.`

	mockDeploymentResponse := *mockDeployment
	mockDeploymentResponse.AirflowVersion = "2.2.4"
	mockDeploymentResponse.DesiredAirflowVersion = "2.2.4"
	mockDeploymentResponse.ClusterID = "ckn4phn1k0104v5xtrer5lpli"

	mockUpdateRequest := map[string]interface{}{
		"deploymentUuid":        mockDeploymentResponse.ID,
		"desiredRuntimeVersion": "4.2.4",
	}

	mockRuntimeReleaseResp := houston.RuntimeReleases{houston.RuntimeRelease{AirflowVersion: "2.2.4", Version: "4.2.4"}}

	api := new(mocks.ClientInterface)
	api.On("GetDeployment", mockDeploymentResponse.ID).Return(&mockDeploymentResponse, nil)
	api.On("UpdateDeploymentRuntime", mockUpdateRequest).Return(&mockDeploymentResponse, nil)
	vars := make(map[string]interface{})
	vars["airflowVersion"] = mockDeploymentResponse.AirflowVersion
	vars["clusterId"] = mockDeploymentResponse.ClusterID
	api.On("GetRuntimeReleases", vars).Return(mockRuntimeReleaseResp, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	houstonClient = api
	output, err := execDeploymentCmd(
		"runtime",
		"migrate",
		"--deployment-id="+mockDeploymentResponse.ID,
	)
	s.NoError(err)
	s.Contains(output, expectedOut)
	api.AssertExpectations(s.T())
}

func (s *Suite) TestDeploymentRuntimeMigrateCancelCommand() {
	appConfig = &houston.AppConfig{
		Flags: houston.FeatureFlags{
			AstroRuntimeEnabled: true,
		},
	}

	expectedOut := `Runtime migrate process has been successfully canceled. Your Deployment was not interrupted and you are still running Airflow 2.2.4.`

	mockDeploymentResponse := *mockDeployment
	mockDeploymentResponse.AirflowVersion = "2.2.4"
	mockDeploymentResponse.DesiredAirflowVersion = "2.2.4"
	mockDeploymentResponse.RuntimeVersion = ""
	mockDeploymentResponse.DesiredRuntimeVersion = "4.2.4"

	mockUpdateRequest := map[string]interface{}{
		"deploymentUuid": mockDeploymentResponse.ID,
	}

	api := new(mocks.ClientInterface)
	api.On("GetDeployment", mockDeploymentResponse.ID).Return(&mockDeploymentResponse, nil)
	api.On("CancelUpdateDeploymentRuntime", mockUpdateRequest).Return(&mockDeploymentResponse, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	houstonClient = api
	output, err := execDeploymentCmd(
		"runtime",
		"migrate",
		"--cancel",
		"--deployment-id="+mockDeploymentResponse.ID,
	)
	s.NoError(err)
	s.Contains(output, expectedOut)
	api.AssertExpectations(s.T())
}
