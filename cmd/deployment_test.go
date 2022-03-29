package cmd

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"os"
	"testing"

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
		CreatedAt: "2021-04-26T20:03:36.262Z",
		UpdatedAt: "2021-04-26T20:03:36.262Z",
	}
	mockAppConfig    = &houston.AppConfig{}
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
	mockDeploymentTeamRole = &houston.RoleBinding{
		Role: houston.DeploymentViewerRole,
		Team: houston.Team{
			ID:   "cl0evnxfl0120dxxu1s4nbnk7",
			Name: "test-team",
		},
		Deployment: houston.Deployment{
			ID:          "ck05r3bor07h40d02y2hw4n4v",
			Label:       "airflow",
			ReleaseName: "airflow",
		},
	}
	mockDeploymentTeam = &houston.Team{
		RoleBindings: []houston.RoleBinding{
			*mockDeploymentTeamRole,
		},
	}
)

func TestDeploymentRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand("deployment")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro deployment")
}

func TestDeploymentCreateCommandNfsMountDisabled(t *testing.T) {
	testUtil.InitTestConfig()
	appConfig := &houston.AppConfig{
		NfsMountDagDeployment: false,
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(appConfig, nil)
	api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--dag-deployment-type=volume", "--nfs-location=test:/test"}, expectedOutput: "", expectedError: "unknown flag: --dag-deployment-type"},
		// test default executor is celery if --executor flag is not provided
		{cmdArgs: []string{"deployment", "create", "new-deployment-name"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
	}
	for _, tt := range myTests {
		output, err := executeCommandC(api, tt.cmdArgs...)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentCreateCommandTriggererDisabled(t *testing.T) {
	testUtil.InitTestConfig()
	appConfigResponse := &houston.AppConfig{TriggererEnabled: false}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(appConfigResponse, nil)
	api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--triggerer-replicas=1"}, expectedOutput: "", expectedError: "unknown flag: --triggerer-replicas"},
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
	}
	for _, tt := range myTests {
		output, err := executeCommandC(api, tt.cmdArgs...)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentCreateCommandTriggererEnabled(t *testing.T) {
	testUtil.InitTestConfig()
	appConfigResponse := &houston.AppConfig{
		TriggererEnabled: true,
		Flags: houston.FeatureFlags{
			TriggererEnabled: true,
		},
	}
	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(appConfigResponse, nil)
	api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--triggerer-replicas=1"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
	}
	for _, tt := range myTests {
		output, err := executeCommandC(api, tt.cmdArgs...)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentCreateCommandNfsMountEnabled(t *testing.T) {
	testUtil.InitTestConfig()
	appConfigResponse := &houston.AppConfig{
		NfsMountDagDeployment: true,
		Flags: houston.FeatureFlags{
			NfsMountDagDeployment: true,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(appConfigResponse, nil)
	api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil).Times(2)

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--dag-deployment-type=volume", "--nfs-location=test:/test"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--dag-deployment-type=dummy"}, expectedOutput: "", expectedError: "please specify the correct DAG deployment type, one of the following: image, volume, git_sync"},
	}
	for _, tt := range myTests {
		output, err := executeCommandC(api, tt.cmdArgs...)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentCreateCommandGitSyncEnabled(t *testing.T) {
	testUtil.InitTestConfig()
	appConfigResponse := &houston.AppConfig{
		Flags: houston.FeatureFlags{
			GitSyncEnabled: true,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(appConfigResponse, nil)
	api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil).Times(5)

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--dag-deployment-type=git_sync", "--git-repository-url=https://github.com/bote795/public-ariflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--sync-interval=200"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=local", "--dag-deployment-type=git_sync", "-u=https://github.com/bote795/public-ariflow-dags-test.git", "-p=dagscopy/", "-b=main", "-s=200"}, expectedOutput: "Successfully created deployment with Local executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--dag-deployment-type=git_sync", "--git-repository-url=git@github.com:bote795/private-ariflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--ssh-key=./testfiles/ssh_key", "--known-hosts=./testfiles/known_hosts"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--dag-deployment-type=git_sync", "--git-repository-url=git@github.com:neel-astro/private-airflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--ssh-key=./testfiles/ssh_key", "--known-hosts=./testfiles/known_hosts"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--dag-deployment-type=git_sync", "--git-repository-url=git@github.com:neel-astro/private-airflow-dags-test.git", "--ssh-key=./testfiles/ssh_key", "--known-hosts=./testfiles/known_hosts"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--dag-deployment-type=dummy"}, expectedOutput: "", expectedError: "please specify the correct DAG deployment type, one of the following: image, volume, git_sync"},
	}
	for _, tt := range myTests {
		output, err := executeCommandC(api, tt.cmdArgs...)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentCreateCommandGitSyncDisabled(t *testing.T) {
	testUtil.InitTestConfig()
	appConfigResponse := &houston.AppConfig{
		Flags: houston.FeatureFlags{GitSyncEnabled: false},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(appConfigResponse, nil)
	api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--dag-deployment-type=git_sync", "--git-repository-url=https://github.com/bote795/public-ariflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--sync-interval=200"}, expectedOutput: "", expectedError: "unknown flag: --dag-deployment-type"},
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
	}
	for _, tt := range myTests {
		output, err := executeCommandC(api, tt.cmdArgs...)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentUpdateTriggererEnabledCommand(t *testing.T) {
	testUtil.InitTestConfig()
	appConfigResponse := &houston.AppConfig{
		TriggererEnabled: true,
		Flags:            houston.FeatureFlags{TriggererEnabled: true},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(appConfigResponse, nil)
	api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil).Twice()

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--triggerer-replicas=1"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
	}
	for _, tt := range myTests {
		output, err := executeCommandC(api, tt.cmdArgs...)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentUpdateCommand(t *testing.T) {
	testUtil.InitTestConfig()
	appConfigResponse := &houston.AppConfig{
		NfsMountDagDeployment: true,
		Flags: houston.FeatureFlags{
			NfsMountDagDeployment: true,
			GitSyncEnabled:        true,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(appConfigResponse, nil)
	api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil).Times(8)

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--dag-deployment-type=volume", "--nfs-location=test:/test"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--dag-deployment-type=git_sync", "--git-repository-url=https://github.com/bote795/public-ariflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--sync-interval=200"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--dag-deployment-type=git_sync", "-u=https://github.com/bote795/public-ariflow-dags-test.git", "-p=dagscopy/", "-b=main", "-s=200"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--dag-deployment-type=git_sync", "--git-repository-url=git@github.com:bote795/private-ariflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--ssh-key=./testfiles/ssh_key", "--known-hosts=./testfiles/known_hosts"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--dag-deployment-type=git_sync", "--git-repository-url=git@github.com:neel-astro/private-airflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--ssh-key=./testfiles/ssh_key", "--known-hosts=./testfiles/known_hosts"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--dag-deployment-type=git_sync", "--git-repository-url=git@github.com:neel-astro/private-airflow-dags-test.git", "--ssh-key=./testfiles/ssh_key", "--known-hosts=./testfiles/known_hosts"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--dag-deployment-type=wrong", "--nfs-location=test:/test"}, expectedOutput: "", expectedError: "please specify the correct DAG deployment type, one of the following: image, volume, git_sync"},
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--executor=local"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "--cloud-role=arn:aws:iam::1234567890:role/test_role4c2301381e"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
	}
	for _, tt := range myTests {
		output, err := executeCommandC(api, tt.cmdArgs...)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentUpdateCommandGitSyncDisabled(t *testing.T) {
	testUtil.InitTestConfig()
	appConfigResponse := &houston.AppConfig{
		NfsMountDagDeployment: true,
		Flags: houston.FeatureFlags{
			NfsMountDagDeployment: true,
			GitSyncEnabled:        false,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(appConfigResponse, nil)
	api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil)

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--dag-deployment-type=volume", "--nfs-location=test:/test"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--dag-deployment-type=git_sync", "--git-repository-url=https://github.com/bote795/public-ariflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--sync-interval=200"}, expectedOutput: "", expectedError: "unknown flag: --git-repository-url"},
	}
	for _, tt := range myTests {
		output, err := executeCommandC(api, tt.cmdArgs...)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentSaRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand("deployment", "service-account")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro deployment service-account")
}

func TestDeploymentSaDeleteWoKeyIdCommand(t *testing.T) {
	testUtil.InitTestConfig()
	_, err := executeCommand("deployment", "service-account", "delete", "--deployment-id=1234")
	assert.Error(t, err)
	assert.EqualError(t, err, "accepts 1 arg(s), received 0")
}

func TestDeploymentSaDeleteWoDeploymentIdCommand(t *testing.T) {
	testUtil.InitTestConfig()
	_, err := executeCommand("deployment", "service-account", "delete", "key-test-id")
	assert.Error(t, err)
	assert.EqualError(t, err, `required flag(s) "deployment-id" not set`)
}

func TestDeploymentSaDeleteRootCommand(t *testing.T) {
	testUtil.InitTestConfig()

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(mockAppConfig, nil)
	api.On("DeleteDeploymentServiceAccount", "1234", mockDeploymentSA.ID).Return(mockDeploymentSA, nil)
	output, err := executeCommandC(api, "deployment", "service-account", "delete", mockDeploymentSA.ID, "--deployment-id=1234")
	assert.NoError(t, err)
	assert.Contains(t, output, "Service Account my_label (q1w2e3r4t5y6u7i8o9p0) successfully deleted")
}

func TestDeploymentSaCreateCommand(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := ` NAME         CATEGORY     ID                       APIKEY                       
 my_label     default      q1w2e3r4t5y6u7i8o9p0     000000000000000000000000     

 Service account successfully created.
`

	mockSA := &houston.DeploymentServiceAccount{
		ID:             mockDeploymentSA.ID,
		APIKey:         mockDeploymentSA.APIKey,
		Label:          mockDeploymentSA.Label,
		Category:       mockDeploymentSA.Category,
		EntityType:     "DEPLOYMENT",
		DeploymentUUID: mockDeployment.ID,
		CreatedAt:      mockDeploymentSA.CreatedAt,
		UpdatedAt:      mockDeploymentSA.UpdatedAt,
		Active:         true,
	}

	expectedSARequest := &houston.CreateServiceAccountRequest{
		DeploymentID: "ck1qg6whg001r08691y117hub",
		Label:        "my_label",
		Category:     "default",
		Role:         houston.DeploymentViewerRole,
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(mockAppConfig, nil)
	api.On("CreateDeploymentServiceAccount", expectedSARequest).Return(mockSA, nil)

	output, err := executeCommandC(api,
		"deployment",
		"service-account",
		"create",
		"--deployment-id="+expectedSARequest.DeploymentID,
		"--label="+expectedSARequest.Label,
		"--role=viewer",
	)
	assert.NoError(t, err)
	assert.Equal(t, expectedOut, output)
}

func TestDeploymentUserAddCommand(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := ` DEPLOYMENT NAME              DEPLOYMENT ID                 USER                       ROLE                  
 prehistoric-gravity-9229     ckggvxkw112212kc9ebv8vu6p     somebody@astronomer.io     DEPLOYMENT_VIEWER     

 Successfully added somebody@astronomer.io as a DEPLOYMENT_VIEWER
`
	expectedAddUserRequest := houston.UpdateDeploymentUserRequest{
		Email:        mockDeploymentUserRole.User.Username,
		Role:         mockDeploymentUserRole.Role,
		DeploymentID: mockDeploymentUserRole.Deployment.ID,
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(mockAppConfig, nil)
	api.On("AddDeploymentUser", expectedAddUserRequest).Return(mockDeploymentUserRole, nil)

	output, err := executeCommandC(api,
		"deployment",
		"user",
		"add",
		"--deployment-id="+mockDeploymentUserRole.Deployment.ID,
		mockDeploymentUserRole.User.Username,
	)
	assert.NoError(t, err)
	assert.Equal(t, expectedOut, output)
}

func TestDeploymentUserDeleteCommand(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := ` DEPLOYMENT ID                 USER                       ROLE                  
 ckggvxkw112212kc9ebv8vu6p     somebody@astronomer.io     DEPLOYMENT_VIEWER     

 Successfully removed the DEPLOYMENT_VIEWER role for somebody@astronomer.io from deployment ckggvxkw112212kc9ebv8vu6p
`

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(mockAppConfig, nil)
	api.On("DeleteDeploymentUser", mockDeploymentUserRole.Deployment.ID, mockDeploymentUserRole.User.Username).
		Return(mockDeploymentUserRole, nil)

	output, err := executeCommandC(api,
		"deployment",
		"user",
		"delete",
		"--deployment-id="+mockDeploymentUserRole.Deployment.ID,
		mockDeploymentUserRole.User.Username,
	)
	assert.NoError(t, err)
	assert.Equal(t, expectedOut, output)
}

func TestDeploymentUserUpdateCommand(t *testing.T) {
	testUtil.InitTestConfig()
	expectedNewRole := houston.DeploymentAdminRole
	expectedOut := `Successfully updated somebody@astronomer.io to a ` + expectedNewRole
	mockResponseUserRole := *mockDeploymentUserRole
	mockResponseUserRole.Role = expectedNewRole

	expectedUpdateUserRequest := houston.UpdateDeploymentUserRequest{
		Email:        mockResponseUserRole.User.Username,
		Role:         expectedNewRole,
		DeploymentID: mockDeploymentUserRole.Deployment.ID,
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(mockAppConfig, nil)
	api.On("UpdateDeploymentUser", expectedUpdateUserRequest).Return(&mockResponseUserRole, nil)

	output, err := executeCommandC(api,
		"deployment",
		"user",
		"update",
		"--deployment-id="+mockResponseUserRole.Deployment.ID,
		"--role="+expectedNewRole,
		mockResponseUserRole.User.Username,
	)
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
}

func TestDeploymentAirflowUpgradeCommand(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := `The upgrade from Airflow 1.10.5 to 1.10.10 has been started. To complete this process, add an Airflow 1.10.10 image to your Dockerfile and deploy to Astronomer.`

	mockDeploymentResponse := *mockDeployment
	mockDeploymentResponse.AirflowVersion = "1.10.5"
	mockDeploymentResponse.DesiredAirflowVersion = "1.10.10"

	mockUpdateRequest := map[string]interface{}{
		"deploymentId":          mockDeploymentResponse.ID,
		"desiredAirflowVersion": mockDeploymentResponse.DesiredAirflowVersion,
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(mockAppConfig, nil)
	api.On("GetDeployment", mockDeploymentResponse.ID).Return(&mockDeploymentResponse, nil)
	api.On("UpdateDeploymentAirflow", mockUpdateRequest).Return(&mockDeploymentResponse, nil)

	output, err := executeCommandC(api,
		"deployment",
		"airflow",
		"upgrade",
		"--deployment-id="+mockDeploymentResponse.ID,
		"--desired-airflow-version="+mockDeploymentResponse.DesiredAirflowVersion,
	)
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
}

func TestDeploymentAirflowUpgradeCancelCommand(t *testing.T) {
	testUtil.InitTestConfig()
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
	api.On("GetAppConfig").Return(mockAppConfig, nil)
	api.On("GetDeployment", mockDeploymentResponse.ID).Return(&mockDeploymentResponse, nil)
	api.On("UpdateDeploymentAirflow", expectedUpdateRequest).Return(&mockDeploymentUpdated, nil)

	output, err := executeCommandC(api,
		"deployment",
		"airflow",
		"upgrade",
		"--cancel",
		"--deployment-id="+mockDeploymentResponse.ID,
	)
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
}

func TestDeploymentSAGetCommand(t *testing.T) {
	testUtil.InitTestConfig()

	mockSA := houston.ServiceAccount{
		ID:        "ckqvfa2cu1468rn9hnr0bqqfk",
		APIKey:    "658b304f36eaaf19860a6d9eb73f7d8a",
		Label:     "yooo can u see me test",
		Category:  "default",
		CreatedAt: "2021-07-08T21:28:57.966Z",
		UpdatedAt: "2021-07-08T21:28:57.966Z",
		Active:    true,
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(mockAppConfig, nil)
	api.On("ListDeploymentServiceAccounts", mockDeployment.ID).Return([]houston.ServiceAccount{mockSA}, nil)

	output, err := executeCommandC(api, "deployment", "sa", "get", "--deployment-id="+mockDeployment.ID)
	assert.NoError(t, err)
	assert.Contains(t, output, mockSA.Label)
	assert.Contains(t, output, mockSA.ID)
	assert.Contains(t, output, mockSA.APIKey)
}

func TestDeploymentDelete(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := `Successfully deleted deployment`

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(mockAppConfig, nil)
	api.On("DeleteDeployment", mockDeployment.ID, false).Return(mockDeployment, nil)

	output, err := executeCommandC(api, "deployment", "delete", mockDeployment.ID)
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
}

func TestDeploymentDeleteHardResponseNo(t *testing.T) {
	testUtil.InitTestConfig()
	appConfig := &houston.AppConfig{
		HardDeleteDeployment: true,
		Flags: houston.FeatureFlags{
			HardDeleteDeployment: true,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(appConfig, nil)

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

	_, err = executeCommandC(api, "deployment", "delete", "--hard", mockDeployment.ID)
	assert.Nil(t, err)
}

func TestDeploymentDeleteHardResponseYes(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := `Successfully deleted deployment`
	appConfig := &houston.AppConfig{
		HardDeleteDeployment: true,
		Flags: houston.FeatureFlags{
			HardDeleteDeployment: true,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(appConfig, nil)
	api.On("DeleteDeployment", mockDeployment.ID, true).Return(mockDeployment, nil)

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

	output, err := executeCommandC(api, "deployment", "delete", "--hard", mockDeployment.ID)
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
}

// Deployment Teams
func TestDeploymentTeamAddCommand(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := ` DEPLOYMENT ID                 TEAM ID                       ROLE                  
 cknz133ra49758zr9w34b87ua     cl0evnxfl0120dxxu1s4nbnk7     DEPLOYMENT_VIEWER     

Successfully added team cl0evnxfl0120dxxu1s4nbnk7 to deployment cknz133ra49758zr9w34b87ua as a DEPLOYMENT_VIEWER
`

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(mockAppConfig, nil)
	api.On("AddDeploymentTeam", mockDeployment.ID, mockDeploymentTeamRole.Team.ID, mockDeploymentTeamRole.Role).Return(mockDeploymentTeamRole, nil)

	output, err := executeCommandC(api,
		"deployment",
		"team",
		"add",
		"--deployment-id="+mockDeployment.ID,
		"--team-id="+mockDeploymentTeamRole.Team.ID,
		"--role="+mockDeploymentTeamRole.Role,
	)
	assert.NoError(t, err)
	assert.Equal(t, expectedOut, output)
}

func TestDeploymentTeamRm(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := ` DEPLOYMENT ID                 TEAM ID                       
 cknz133ra49758zr9w34b87ua     cl0evnxfl0120dxxu1s4nbnk7     

 Successfully removed team cl0evnxfl0120dxxu1s4nbnk7 from deployment cknz133ra49758zr9w34b87ua
`

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(mockAppConfig, nil)
	api.On("RemoveDeploymentTeam", mockDeployment.ID, mockDeploymentTeamRole.Team.ID).Return(mockDeploymentTeamRole, nil)
	output, err := executeCommandC(api,
		"deployment",
		"team",
		"remove",
		mockDeploymentTeamRole.Team.ID,
		"--deployment-id="+mockDeployment.ID,
	)

	assert.NoError(t, err)
	assert.Equal(t, expectedOut, output)
}

func TestDeploymentTeamUpdateCommand(t *testing.T) {
	testUtil.InitTestConfig()

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(mockAppConfig, nil)
	api.On("GetDeploymentTeamRole", mockDeployment.ID, mockDeploymentTeamRole.Team.ID).Return(mockDeploymentTeam, nil)
	api.On("UpdateDeploymentTeamRole", mockDeployment.ID, mockDeploymentTeamRole.Team.ID, mockDeploymentTeamRole.Role).Return(mockDeploymentTeamRole, nil)

	_, err := executeCommandC(api,
		"deployment",
		"team",
		"update",
		mockDeploymentTeamRole.Team.ID,
		"--deployment-id="+mockDeployment.ID,
		"--role="+mockDeploymentTeamRole.Role,
	)
	assert.NoError(t, err)
}

func TestDeploymentTeamsListCmd(t *testing.T) {
	testUtil.InitTestConfig()
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString("")),
			Header:     make(http.Header),
		}
	})
	houstonClient = houston.NewClient(client)
	buf := new(bytes.Buffer)
	cmd := newDeploymentTeamListCmd(buf)
	assert.NotNil(t, cmd)
	assert.Nil(t, cmd.Args)
}
