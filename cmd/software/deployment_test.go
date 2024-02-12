package software

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
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

func TestDeploymentRootCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	output, err := execDeploymentCmd()
	assert.NoError(t, err)
	assert.Contains(t, output, "deployment [command]")
}

func TestDeploymentCreateCommandNfsMountDisabled(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	appConfig = &houston.AppConfig{
		NfsMountDagDeployment: false,
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(appConfig, nil)
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
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentCreateWithTypeDagDeploy(t *testing.T) {
	t.Run("user should not be prompted if deployment type is not dag_deploy", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		appConfig = &houston.AppConfig{
			TriggererEnabled: true,
			Flags: houston.FeatureFlags{
				TriggererEnabled: true,
			},
		}
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(appConfig, nil)
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

		cmdArgs := []string{"create", "--label=new-deployment-name", "--executor=celery", "--triggerer-replicas=1"}
		expectedOutput := "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs"
		houstonClient = api
		output, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, output, expectedOutput)
		api.AssertExpectations(t)
	})

	t.Run("user should not be prompted if deployment type is dag_deploy but -f flag is sent", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		appConfig = &houston.AppConfig{
			TriggererEnabled: true,
			Flags: houston.FeatureFlags{
				TriggererEnabled:  true,
				DagOnlyDeployment: true,
			},
		}
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(appConfig, nil)
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

		cmdArgs := []string{"create", "--label=new-deployment-name", "--executor=celery", "--dag-deployment-type=dag_deploy", "--triggerer-replicas=1", "--force"}
		expectedOutput := "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs"
		houstonClient = api
		output, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, output, expectedOutput)
		api.AssertExpectations(t)
	})

	t.Run("user should be prompted if deployment type is dag_deploy and -f flag is not sent. No deployment is created without confirmation.", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		appConfig = &houston.AppConfig{
			TriggererEnabled: true,
			Flags: houston.FeatureFlags{
				TriggererEnabled:  true,
				DagOnlyDeployment: true,
			},
		}

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(appConfig, nil)
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)
		cmdArgs := []string{"create", "--label=new-deployment-name", "--executor=celery", "--dag-deployment-type=dag_deploy", "--triggerer-replicas=1"}
		houstonClient = api

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		defer testUtil.MockUserInput(t, "n")()

		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		// Houston CreateDeployment API should not be called if user does not confirm the prompt
		api.AssertNotCalled(t, "CreateDeployment", mock.Anything)
	})

	t.Run("user should be prompted if deployment type is dag_deploy and -f flag is not sent. Deployment is created with confirmation.", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		appConfig = &houston.AppConfig{
			TriggererEnabled: true,
			Flags: houston.FeatureFlags{
				TriggererEnabled:  true,
				DagOnlyDeployment: true,
			},
		}

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(appConfig, nil)
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)
		cmdArgs := []string{"create", "--label=new-deployment-name", "--executor=celery", "--dag-deployment-type=dag_deploy", "--triggerer-replicas=1"}
		houstonClient = api

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		defer testUtil.MockUserInput(t, "y")()

		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		// Houston CreateDeployment API should be called if user confirms the prompt
		api.AssertCalled(t, "CreateDeployment", mock.Anything)
	})
}

func TestDeploymentCreateCommandTriggererDisabled(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	appConfig = &houston.AppConfig{TriggererEnabled: false}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(appConfig, nil)
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
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentCreateCommandTriggererEnabled(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	appConfig = &houston.AppConfig{
		TriggererEnabled: true,
		Flags: houston.FeatureFlags{
			TriggererEnabled: true,
		},
	}
	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(appConfig, nil)
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
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentCreateCommandNfsMountEnabled(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	appConfig = &houston.AppConfig{
		NfsMountDagDeployment: true,
		Flags: houston.FeatureFlags{
			NfsMountDagDeployment: true,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(appConfig, nil)
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
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentCreateCommandGitSyncEnabled(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	appConfig = &houston.AppConfig{
		Flags: houston.FeatureFlags{
			GitSyncEnabled: true,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(appConfig, nil)
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
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentCreateCommandDagOnlyDeployEnabled(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	appConfig = &houston.AppConfig{
		Flags: houston.FeatureFlags{
			DagOnlyDeployment: true,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(appConfig, nil)
	api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil).Times(5)

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=celery", "--dag-deployment-type=dag_deploy"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"create", "--label=new-deployment-name", "--executor=celery", "--dag-deployment-type=dummy"}, expectedOutput: "", expectedError: ErrInvalidDAGDeploymentType.Error()},
	}
	for _, tt := range myTests {
		houstonClient = api
		output, err := execDeploymentCmd(tt.cmdArgs...)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentCreateCommandGitSyncDisabled(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	appConfig = &houston.AppConfig{
		Flags: houston.FeatureFlags{GitSyncEnabled: false},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(appConfig, nil)
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
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentUpdateTriggererEnabledCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	appConfig = &houston.AppConfig{
		TriggererEnabled: true,
		Flags:            houston.FeatureFlags{TriggererEnabled: true},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(appConfig, nil)
	api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil).Twice()

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
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentUpdateWithTypeDagDeploy(t *testing.T) {
	t.Run("user should not be prompted if new deployment type is not dag_deploy", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		appConfig = &houston.AppConfig{
			Flags: houston.FeatureFlags{
				DagOnlyDeployment: true,
			},
		}

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(appConfig, nil)
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
		assert.NoError(t, err)
		assert.Contains(t, output, "Successfully updated deployment")
	})

	t.Run("user should not be prompted if new deployment type is dag_deploy and current deployment type is also dag_deploy", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		appConfig = &houston.AppConfig{
			Flags: houston.FeatureFlags{
				DagOnlyDeployment: true,
			},
		}

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(appConfig, nil)
		api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil)
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
		assert.NoError(t, err)
		assert.Contains(t, output, "Successfully updated deployment")
	})

	t.Run("user should be prompted if new deployment type is dag_deploy and current deployment type is not dag_deploy. User rejects.", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		appConfig = &houston.AppConfig{
			Flags: houston.FeatureFlags{
				DagOnlyDeployment: true,
			},
		}

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(appConfig, nil)
		api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil)
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
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		defer testUtil.MockUserInput(t, "n")()

		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		api.AssertCalled(t, "GetDeployment", mock.Anything)
		// Houston UpdateDeployment API should not be called if user does not confirm the prompt
		api.AssertNotCalled(t, "UpdateDeployment", mock.Anything)
	})

	t.Run("user should be prompted if new deployment type is dag_deploy and current deployment type is not dag_deploy. User confirms.", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		appConfig = &houston.AppConfig{
			Flags: houston.FeatureFlags{
				DagOnlyDeployment: true,
			},
		}

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(appConfig, nil)
		api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil)
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
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		defer testUtil.MockUserInput(t, "y")()

		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		api.AssertCalled(t, "GetDeployment", mock.Anything)
		// Houston UpdateDeployment API should be called if user confirms the prompt
		api.AssertCalled(t, "UpdateDeployment", mock.Anything)
	})
}

func TestDeploymentUpdateCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	appConfig = &houston.AppConfig{
		NfsMountDagDeployment: true,
		Flags: houston.FeatureFlags{
			NfsMountDagDeployment: true,
			GitSyncEnabled:        true,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(appConfig, nil)
	api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil).Times(8)

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
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentUpdateCommandGitSyncDisabled(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	appConfig = &houston.AppConfig{
		NfsMountDagDeployment: true,
		Flags: houston.FeatureFlags{
			NfsMountDagDeployment: true,
			GitSyncEnabled:        false,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(appConfig, nil)
	api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil)

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
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentUpdateCommandDagOnlyDeployEnabled(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	appConfig = &houston.AppConfig{
		Flags: houston.FeatureFlags{
			DagOnlyDeployment: true,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(appConfig, nil)
	api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil)

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--dag-deployment-type=dag_deploy"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"update", "cknrml96n02523xr97ygj95n5", "--label=test22222", "--dag-deployment-type=invalid"}, expectedOutput: "", expectedError: ErrInvalidDAGDeploymentType.Error()},
	}
	for _, tt := range myTests {
		houstonClient = api
		output, err := execDeploymentCmd(tt.cmdArgs...)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentAirflowUpgradeCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	expectedOut := `The upgrade from Airflow 1.10.5 to 1.10.10 has been started. To complete this process, add an Airflow 1.10.10 image to your Dockerfile and deploy to Astronomer.`

	mockDeploymentResponse := *mockDeployment
	mockDeploymentResponse.AirflowVersion = "1.10.5"
	mockDeploymentResponse.DesiredAirflowVersion = "1.10.10"

	mockUpdateRequest := map[string]interface{}{
		"deploymentId":          mockDeploymentResponse.ID,
		"desiredAirflowVersion": mockDeploymentResponse.DesiredAirflowVersion,
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
	api.On("GetDeployment", mockDeploymentResponse.ID).Return(&mockDeploymentResponse, nil)
	api.On("UpdateDeploymentAirflow", mockUpdateRequest).Return(&mockDeploymentResponse, nil)

	houstonClient = api
	output, err := execDeploymentCmd(
		"airflow",
		"upgrade",
		"--deployment-id="+mockDeploymentResponse.ID,
		"--desired-airflow-version="+mockDeploymentResponse.DesiredAirflowVersion,
	)
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
}

func TestDeploymentAirflowUpgradeCancelCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
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
	api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
	api.On("GetDeployment", mockDeploymentResponse.ID).Return(&mockDeploymentResponse, nil)
	api.On("UpdateDeploymentAirflow", expectedUpdateRequest).Return(&mockDeploymentUpdated, nil)

	houstonClient = api
	output, err := execDeploymentCmd(
		"airflow",
		"upgrade",
		"--cancel",
		"--deployment-id="+mockDeploymentResponse.ID,
	)
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
}

func TestDeploymentDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	expectedOut := `Successfully deleted deployment`

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
	api.On("DeleteDeployment", houston.DeleteDeploymentRequest{DeploymentID: mockDeployment.ID, HardDelete: false}).Return(mockDeployment, nil)

	houstonClient = api
	output, err := execDeploymentCmd("delete", mockDeployment.ID)
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
}

func TestDeploymentList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	api := new(mocks.ClientInterface)
	api.On("ListDeployments", houston.ListDeploymentsRequest{}).Return([]houston.Deployment{*mockDeployment}, nil)
	allDeployments = true

	houstonClient = api
	output, err := execDeploymentCmd("list", "--all")
	assert.NoError(t, err)
	assert.Contains(t, output, mockDeployment.ID)
	api.AssertExpectations(t)
}

func TestDeploymentDeleteHardResponseNo(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	appConfig = &houston.AppConfig{
		HardDeleteDeployment: true,
		Flags: houston.FeatureFlags{
			HardDeleteDeployment: true,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(appConfig, nil)

	// mock os.Stdin
	input := []byte("n")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Write(input)
	if err != nil {
		t.Error(err)
	}
	w.Close()
	stdin := os.Stdin
	// Restore stdin right after the test.
	defer func() { os.Stdin = stdin }()
	os.Stdin = r

	houstonClient = api
	_, err = execDeploymentCmd("delete", "--hard", mockDeployment.ID)
	assert.Nil(t, err)
}

func TestDeploymentDeleteHardResponseYes(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	expectedOut := `Successfully deleted deployment`
	appConfig = &houston.AppConfig{
		HardDeleteDeployment: true,
		Flags: houston.FeatureFlags{
			HardDeleteDeployment: true,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(appConfig, nil)
	api.On("DeleteDeployment", houston.DeleteDeploymentRequest{DeploymentID: mockDeployment.ID, HardDelete: true}).Return(mockDeployment, nil)

	// mock os.Stdin
	input := []byte("y")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Write(input)
	if err != nil {
		t.Error(err)
	}
	w.Close()
	stdin := os.Stdin
	// Restore stdin right after the test.
	defer func() { os.Stdin = stdin }()
	os.Stdin = r

	houstonClient = api
	output, err := execDeploymentCmd("delete", "--hard", mockDeployment.ID)
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
}

func TestDeploymentRuntimeUpgradeCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

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

	houstonClient = api
	output, err := execDeploymentCmd(
		"runtime",
		"upgrade",
		"--deployment-id="+mockDeploymentResponse.ID,
		"--desired-runtime-version="+mockDeploymentResponse.DesiredRuntimeVersion,
	)
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
}

func TestDeploymentRuntimeUpgradeCancelCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

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

	houstonClient = api
	output, err := execDeploymentCmd(
		"runtime",
		"upgrade",
		"--cancel",
		"--deployment-id="+mockDeploymentResponse.ID,
	)
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
	api.AssertExpectations(t)
}

func TestDeploymentRuntimeMigrateCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

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
	api.On("GetRuntimeReleases", mockDeploymentResponse.AirflowVersion).Return(mockRuntimeReleaseResp, nil)

	houstonClient = api
	output, err := execDeploymentCmd(
		"runtime",
		"migrate",
		"--deployment-id="+mockDeploymentResponse.ID,
	)
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
	api.AssertExpectations(t)
}

func TestDeploymentRuntimeMigrateCancelCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

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

	houstonClient = api
	output, err := execDeploymentCmd(
		"runtime",
		"migrate",
		"--cancel",
		"--deployment-id="+mockDeploymentResponse.ID,
	)
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
	api.AssertExpectations(t)
}
