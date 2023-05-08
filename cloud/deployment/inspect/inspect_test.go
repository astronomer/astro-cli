package inspect

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/astro-client"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/context"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

var (
	errGetDeployment = errors.New("test get deployment error")
	errMarshal       = errors.New("test error")
)

type Suite struct {
	suite.Suite
}

func TestCloudDeploymentInspectSuite(t *testing.T) {
	suite.Run(t, new(Suite))
}

func errReturningYAMLMarshal(v interface{}) ([]byte, error) {
	return []byte{}, errMarshal
}

func errReturningJSONMarshal(v interface{}, prefix, indent string) ([]byte, error) {
	return []byte{}, errMarshal
}

func errorReturningDecode(input, output interface{}) error {
	return errMarshal
}

func restoreDecode(replace func(input, output interface{}) error) {
	decodeToStruct = replace
}

func restoreJSONMarshal(replace func(v interface{}, prefix, indent string) ([]byte, error)) {
	jsonMarshal = replace
}

func restoreYAMLMarshal(replace func(v interface{}) ([]byte, error)) {
	yamlMarshal = replace
}

func (s *Suite) TestInspect() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	workspaceID := "test-ws-id"
	deploymentID := "test-deployment-id"
	deploymentName := "test-deployment-label"
	deploymentResponse := []astro.Deployment{
		{
			ID:          deploymentID,
			Label:       deploymentName,
			ReleaseName: "great-release-name",
			Workspace:   astro.Workspace{ID: workspaceID},
			Cluster: astro.Cluster{
				ID: "cluster-id",
				NodePools: []astro.NodePool{
					{
						ID:               "test-pool-id",
						IsDefault:        false,
						NodeInstanceType: "test-instance-type",
						CreatedAt:        time.Now(),
					},
					{
						ID:               "test-pool-id-1",
						IsDefault:        true,
						NodeInstanceType: "test-instance-type-1",
						CreatedAt:        time.Now(),
					},
				},
			},
			RuntimeRelease: astro.RuntimeRelease{Version: "6.0.0", AirflowVersion: "2.4.0"},
			DeploymentSpec: astro.DeploymentSpec{
				Executor: "CeleryExecutor",
				Scheduler: astro.Scheduler{
					AU:       5,
					Replicas: 3,
				},
				Webserver: astro.Webserver{URL: "some-url"},
			},
			WorkerQueues: []astro.WorkerQueue{
				{
					ID:                "test-wq-id",
					Name:              "default",
					IsDefault:         true,
					MaxWorkerCount:    130,
					MinWorkerCount:    12,
					WorkerConcurrency: 110,
					NodePoolID:        "test-pool-id",
				},
				{
					ID:                "test-wq-id-1",
					Name:              "test-queue-1",
					IsDefault:         false,
					MaxWorkerCount:    175,
					MinWorkerCount:    8,
					WorkerConcurrency: 150,
					NodePoolID:        "test-pool-id-1",
				},
			},
			UpdatedAt: time.Now(),
			Status:    "HEALTHY",
		},
		{
			ID:             "test-deployment-id-1",
			Label:          "test-deployment-label-1",
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{
				Scheduler: astro.Scheduler{
					AU:       5,
					Replicas: 3,
				},
			},
			WorkerQueues: []astro.WorkerQueue{
				{
					ID:                "test-wq-id-2",
					Name:              "test-queue-2",
					IsDefault:         false,
					MaxWorkerCount:    130,
					MinWorkerCount:    12,
					WorkerConcurrency: 110,
					NodePoolID:        "test-nodepool-id-2",
				},
				{
					ID:                "test-wq-id-3",
					Name:              "test-queue-3",
					IsDefault:         true,
					MaxWorkerCount:    175,
					MinWorkerCount:    8,
					WorkerConcurrency: 150,
					NodePoolID:        "test-nodepool-id-3",
				},
			},
		},
	}
	s.Run("prints a deployment in yaml format to stdout", func() {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", mock.Anything, workspaceID).Return(deploymentResponse, nil).Once()
		err := Inspect(workspaceID, "", deploymentID, "yaml", mockClient, out, "", false)
		s.NoError(err)
		s.Contains(out.String(), deploymentResponse[0].ReleaseName)
		s.Contains(out.String(), deploymentName)
		s.Contains(out.String(), deploymentResponse[0].RuntimeRelease.Version)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("prints a deployment template in yaml format to stdout", func() {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", mock.Anything, workspaceID).Return(deploymentResponse, nil).Once()
		err := Inspect(workspaceID, "", deploymentID, "yaml", mockClient, out, "", true)
		s.NoError(err)
		s.Contains(out.String(), deploymentResponse[0].RuntimeRelease.Version)
		s.NotContains(out.String(), deploymentResponse[0].ReleaseName)
		s.NotContains(out.String(), deploymentName)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("prints a deployment in json format to stdout", func() {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", mock.Anything, workspaceID).Return(deploymentResponse, nil).Once()
		err := Inspect(workspaceID, "", deploymentID, "json", mockClient, out, "", false)
		s.NoError(err)
		s.Contains(out.String(), deploymentResponse[0].ReleaseName)
		s.Contains(out.String(), deploymentName)
		s.Contains(out.String(), deploymentResponse[0].RuntimeRelease.Version)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("prints a deployment template in json format to stdout", func() {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", mock.Anything, workspaceID).Return(deploymentResponse, nil).Once()
		err := Inspect(workspaceID, "", deploymentID, "json", mockClient, out, "", true)
		s.NoError(err)
		s.Contains(out.String(), deploymentResponse[0].RuntimeRelease.Version)
		s.NotContains(out.String(), deploymentResponse[0].ReleaseName)
		s.NotContains(out.String(), deploymentName)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("prints a deployment's specific field to stdout", func() {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", mock.Anything, workspaceID).Return(deploymentResponse, nil).Once()
		err := Inspect(workspaceID, "", deploymentID, "yaml", mockClient, out, "configuration.cluster_name", false)
		s.NoError(err)
		s.Contains(out.String(), deploymentResponse[0].Cluster.Name)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("prompts for a deployment to inspect if no deployment name or id was provided", func() {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		defer testUtil.MockUserInput(s.T(), "1")() // selecting test-deployment-id
		mockClient.On("ListDeployments", mock.Anything, workspaceID).Return(deploymentResponse, nil).Once()
		err := Inspect(workspaceID, "", "", "yaml", mockClient, out, "", false)
		s.NoError(err)
		s.Contains(out.String(), deploymentName)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if listing deployment fails", func() {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", mock.Anything, workspaceID).Return([]astro.Deployment{}, errGetDeployment).Once()
		err := Inspect(workspaceID, "", deploymentID, "yaml", mockClient, out, "", false)
		s.ErrorIs(err, errGetDeployment)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if requested field is not found in deployment", func() {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", mock.Anything, workspaceID).Return(deploymentResponse, nil).Once()
		err := Inspect(workspaceID, "", deploymentID, "yaml", mockClient, out, "no-exist-information", false)
		s.ErrorIs(err, errKeyNotFound)
		s.Equal("", out.String())
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if formatting deployment fails", func() {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		originalMarshal := yamlMarshal
		yamlMarshal = errReturningYAMLMarshal
		defer restoreYAMLMarshal(originalMarshal)
		mockClient.On("ListDeployments", mock.Anything, workspaceID).Return(deploymentResponse, nil).Once()
		err := Inspect(workspaceID, "", deploymentID, "yaml", mockClient, out, "", false)
		s.ErrorIs(err, errMarshal)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if getting context fails", func() {
		testUtil.InitTestConfig(testUtil.ErrorReturningContext)
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		err := Inspect(workspaceID, "", deploymentID, "yaml", mockClient, out, "", false)
		s.ErrorContains(err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
		mockClient.AssertExpectations(s.T())
	})
	s.Run("Hide Cluster Info if an org is hosted", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := context.GetCurrentContext()
		s.NoError(err)
		ctx.SetContextKey("organization_product", "HOSTED")
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", mock.Anything, workspaceID).Return(deploymentResponse, nil).Once()
		err = Inspect(workspaceID, "", deploymentID, "yaml", mockClient, out, "", false)
		s.NoError(err)
		s.Contains(out.String(), deploymentResponse[0].ReleaseName)
		s.Contains(out.String(), deploymentName)
		s.Contains(out.String(), deploymentResponse[0].RuntimeRelease.Version)
		s.Contains(out.String(), "N/A")
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestGetDeploymentInspectInfo() {
	sourceDeployment := astro.Deployment{
		ID:          "test-deployment-id",
		Label:       "test-deployment-label",
		Workspace:   astro.Workspace{ID: "test-ws-id"},
		ReleaseName: "great-release-name",
		Cluster: astro.Cluster{
			ID: "cluster-id",
			NodePools: []astro.NodePool{
				{
					ID:               "test-pool-id",
					IsDefault:        false,
					NodeInstanceType: "test-instance-type",
					CreatedAt:        time.Now(),
				},
				{
					ID:               "test-pool-id-1",
					IsDefault:        true,
					NodeInstanceType: "test-instance-type-1",
					CreatedAt:        time.Now(),
				},
			},
		},
		DagDeployEnabled: true,
		RuntimeRelease:   astro.RuntimeRelease{Version: "6.0.0", AirflowVersion: "2.4.0"},
		DeploymentSpec: astro.DeploymentSpec{
			Executor: "CeleryExecutor",
			Scheduler: astro.Scheduler{
				AU:       5,
				Replicas: 3,
			},
			Webserver: astro.Webserver{URL: "some-url"},
			Image: astro.Image{
				Tag: "some-tag",
			},
		},
		WorkerQueues: []astro.WorkerQueue{
			{
				ID:                "test-wq-id",
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    130,
				MinWorkerCount:    12,
				WorkerConcurrency: 110,
				NodePoolID:        "test-pool-id",
			},
			{
				ID:                "test-wq-id-1",
				Name:              "test-queue-1",
				IsDefault:         false,
				MaxWorkerCount:    175,
				MinWorkerCount:    8,
				WorkerConcurrency: 150,
				NodePoolID:        "test-pool-id-1",
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Status:    "HEALTHY",
	}

	s.Run("returns deployment metadata for the requested cloud deployment", func() {
		var actualDeploymentMeta deploymentMetadata
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		expectedCloudDomainURL := "cloud.astronomer.io/" + sourceDeployment.Workspace.ID +
			"/deployments/" + sourceDeployment.ID + "/analytics"
		expectedDeploymentMetadata := deploymentMetadata{
			DeploymentID:   &sourceDeployment.ID,
			WorkspaceID:    &sourceDeployment.Workspace.ID,
			ClusterID:      &sourceDeployment.Cluster.ID,
			AirflowVersion: &sourceDeployment.RuntimeRelease.AirflowVersion,
			CurrentTag:     &sourceDeployment.DeploymentSpec.Image.Tag,
			ReleaseName:    &sourceDeployment.ReleaseName,
			DeploymentURL:  &expectedCloudDomainURL,
			WebserverURL:   &sourceDeployment.DeploymentSpec.Webserver.URL,
			CreatedAt:      &sourceDeployment.CreatedAt,
			UpdatedAt:      &sourceDeployment.UpdatedAt,
			Status:         &sourceDeployment.Status,
		}
		rawDeploymentInfo, err := getDeploymentInfo(&sourceDeployment)
		s.NoError(err)
		err = decodeToStruct(rawDeploymentInfo, &actualDeploymentMeta)
		s.NoError(err)
		s.Equal(expectedDeploymentMetadata, actualDeploymentMeta)
	})
	s.Run("returns deployment metadata for the requested local deployment", func() {
		var actualDeploymentMeta deploymentMetadata
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		expectedCloudDomainURL := "localhost:5000/" + sourceDeployment.Workspace.ID +
			"/deployments/" + sourceDeployment.ID + "/analytics"
		expectedDeploymentMetadata := deploymentMetadata{
			DeploymentID:   &sourceDeployment.ID,
			WorkspaceID:    &sourceDeployment.Workspace.ID,
			ClusterID:      &sourceDeployment.Cluster.ID,
			ReleaseName:    &sourceDeployment.ReleaseName,
			AirflowVersion: &sourceDeployment.RuntimeRelease.AirflowVersion,
			CurrentTag:     &sourceDeployment.DeploymentSpec.Image.Tag,
			Status:         &sourceDeployment.Status,
			CreatedAt:      &sourceDeployment.CreatedAt,
			UpdatedAt:      &sourceDeployment.UpdatedAt,
			DeploymentURL:  &expectedCloudDomainURL,
			WebserverURL:   &sourceDeployment.DeploymentSpec.Webserver.URL,
		}
		rawDeploymentInfo, err := getDeploymentInfo(&sourceDeployment)
		s.NoError(err)
		err = decodeToStruct(rawDeploymentInfo, &actualDeploymentMeta)
		s.NoError(err)
		s.Equal(expectedDeploymentMetadata, actualDeploymentMeta)
	})
	s.Run("returns error if getting context fails", func() {
		var actualDeploymentMeta deploymentMetadata
		// get an error from GetCurrentContext()
		testUtil.InitTestConfig(testUtil.ErrorReturningContext)
		expectedDeploymentMetadata := deploymentMetadata{}
		rawDeploymentInfo, err := getDeploymentInfo(&sourceDeployment)
		s.ErrorContains(err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
		err = decodeToStruct(rawDeploymentInfo, &actualDeploymentMeta)
		s.NoError(err)
		s.Equal(expectedDeploymentMetadata, actualDeploymentMeta)
	})
}

func (s *Suite) TestGetDeploymentConfig() {
	sourceDeployment := astro.Deployment{
		ID:          "test-deployment-id",
		Label:       "test-deployment-label",
		Description: "description",
		Workspace:   astro.Workspace{ID: "test-ws-id", Label: "test-ws"},
		ReleaseName: "great-release-name",
		AlertEmails: []string{"email1", "email2"},
		Cluster: astro.Cluster{
			ID:   "cluster-id",
			Name: "test-cluster",
			NodePools: []astro.NodePool{
				{
					ID:               "test-pool-id",
					IsDefault:        false,
					NodeInstanceType: "test-instance-type",
					CreatedAt:        time.Now(),
				},
				{
					ID:               "test-pool-id-1",
					IsDefault:        true,
					NodeInstanceType: "test-instance-type-1",
					CreatedAt:        time.Now(),
				},
			},
		},
		RuntimeRelease: astro.RuntimeRelease{Version: "6.0.0", AirflowVersion: "2.4.0"},
		DeploymentSpec: astro.DeploymentSpec{
			Executor: "CeleryExecutor",
			Scheduler: astro.Scheduler{
				AU:       5,
				Replicas: 3,
			},
			Webserver: astro.Webserver{URL: "some-url"},
			EnvironmentVariablesObjects: []astro.EnvironmentVariablesObject{
				{
					Key:       "foo",
					Value:     "bar",
					IsSecret:  false,
					UpdatedAt: "NOW",
				},
				{
					Key:       "bar",
					Value:     "baz",
					IsSecret:  true,
					UpdatedAt: "NOW+1",
				},
			},
		},
		WorkerQueues: []astro.WorkerQueue{
			{
				ID:                "test-wq-id",
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    130,
				MinWorkerCount:    12,
				WorkerConcurrency: 110,
				NodePoolID:        "test-pool-id",
			},
			{
				ID:                "test-wq-id-1",
				Name:              "test-queue-1",
				IsDefault:         false,
				MaxWorkerCount:    175,
				MinWorkerCount:    8,
				WorkerConcurrency: 150,
				NodePoolID:        "test-pool-id-1",
			},
		},
		UpdatedAt:        time.Now(),
		Status:           "UNHEALTHY",
		DagDeployEnabled: true,
	}

	s.Run("returns deployment config for the requested cloud deployment", func() {
		var actualDeploymentConfig deploymentConfig
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		expectedDeploymentConfig := deploymentConfig{
			Name:             sourceDeployment.Label,
			Description:      sourceDeployment.Description,
			WorkspaceName:    sourceDeployment.Workspace.Label,
			ClusterName:      sourceDeployment.Cluster.Name,
			RunTimeVersion:   sourceDeployment.RuntimeRelease.Version,
			SchedulerAU:      sourceDeployment.DeploymentSpec.Scheduler.AU,
			SchedulerCount:   sourceDeployment.DeploymentSpec.Scheduler.Replicas,
			DagDeployEnabled: sourceDeployment.DagDeployEnabled,
			Executor:         sourceDeployment.DeploymentSpec.Executor,
		}
		rawDeploymentConfig := getDeploymentConfig(&sourceDeployment)
		err := decodeToStruct(rawDeploymentConfig, &actualDeploymentConfig)
		s.NoError(err)
		s.Equal(expectedDeploymentConfig, actualDeploymentConfig)
	})
}

func (s *Suite) TestGetPrintableDeployment() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	sourceDeployment := astro.Deployment{
		ID:          "test-deployment-id",
		Label:       "test-deployment-label",
		Description: "description",
		Workspace:   astro.Workspace{ID: "test-ws-id", Label: "test-ws"},
		ReleaseName: "great-release-name",
		AlertEmails: []string{"email1", "email2"},
		Cluster: astro.Cluster{
			ID:   "cluster-id",
			Name: "test-cluster",
			NodePools: []astro.NodePool{
				{
					ID:               "test-pool-id",
					IsDefault:        false,
					NodeInstanceType: "test-instance-type",
					CreatedAt:        time.Now(),
				},
				{
					ID:               "test-pool-id-1",
					IsDefault:        true,
					NodeInstanceType: "test-instance-type-1",
					CreatedAt:        time.Now(),
				},
			},
		},
		RuntimeRelease: astro.RuntimeRelease{Version: "6.0.0", AirflowVersion: "2.4.0"},
		DeploymentSpec: astro.DeploymentSpec{
			Executor: "CeleryExecutor",
			Scheduler: astro.Scheduler{
				AU:       5,
				Replicas: 3,
			},
			Webserver: astro.Webserver{URL: "some-url"},
			EnvironmentVariablesObjects: []astro.EnvironmentVariablesObject{
				{
					Key:       "foo",
					Value:     "bar",
					IsSecret:  false,
					UpdatedAt: "NOW",
				},
				{
					Key:       "bar",
					Value:     "baz",
					IsSecret:  true,
					UpdatedAt: "NOW+1",
				},
			},
		},
		WorkerQueues: []astro.WorkerQueue{
			{
				ID:                "test-wq-id",
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    130,
				MinWorkerCount:    12,
				WorkerConcurrency: 110,
				NodePoolID:        "test-pool-id",
			},
			{
				ID:                "test-wq-id-1",
				Name:              "test-queue-1",
				IsDefault:         false,
				MaxWorkerCount:    175,
				MinWorkerCount:    8,
				WorkerConcurrency: 150,
				NodePoolID:        "test-pool-id-1",
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Status:    "UNHEALTHY",
	}
	s.Run("returns a deployment map", func() {
		info, _ := getDeploymentInfo(&sourceDeployment)
		config := getDeploymentConfig(&sourceDeployment)
		additional := getAdditional(&sourceDeployment)
		expectedDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
			},
		}
		actualDeployment := getPrintableDeployment(info, config, additional)
		s.Equal(expectedDeployment, actualDeployment)
	})
}

func (s *Suite) TestGetAdditional() {
	sourceDeployment := astro.Deployment{
		ID:          "test-deployment-id",
		Label:       "test-deployment-label",
		Description: "description",
		Workspace:   astro.Workspace{ID: "test-ws-id"},
		ReleaseName: "great-release-name",
		AlertEmails: []string{"email1", "email2"},
		Cluster: astro.Cluster{
			ID: "cluster-id",
			NodePools: []astro.NodePool{
				{
					ID:               "test-pool-id",
					IsDefault:        false,
					NodeInstanceType: "test-instance-type",
					CreatedAt:        time.Now(),
				},
				{
					ID:               "test-pool-id-1",
					IsDefault:        true,
					NodeInstanceType: "test-instance-type-1",
					CreatedAt:        time.Now(),
				},
			},
		},
		RuntimeRelease: astro.RuntimeRelease{Version: "6.0.0", AirflowVersion: "2.4.0"},
		DeploymentSpec: astro.DeploymentSpec{
			Executor: "CeleryExecutor",
			Scheduler: astro.Scheduler{
				AU:       5,
				Replicas: 3,
			},
			Webserver: astro.Webserver{URL: "some-url"},
			EnvironmentVariablesObjects: []astro.EnvironmentVariablesObject{
				{
					Key:       "foo",
					Value:     "bar",
					IsSecret:  false,
					UpdatedAt: time.Now().String(),
				},
				{
					Key:       "bar",
					Value:     "baz",
					IsSecret:  true,
					UpdatedAt: time.Now().String(),
				},
			},
		},
		WorkerQueues: []astro.WorkerQueue{
			{
				ID:                "test-wq-id",
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    130,
				MinWorkerCount:    12,
				WorkerConcurrency: 110,
				NodePoolID:        "test-pool-id",
				PodCPU:            "SmallCPU",
				PodRAM:            "megsOfRam",
			},
			{
				ID:                "test-wq-id-1",
				Name:              "test-queue-1",
				IsDefault:         false,
				MaxWorkerCount:    175,
				MinWorkerCount:    8,
				WorkerConcurrency: 150,
				NodePoolID:        "test-pool-id-1",
				PodCPU:            "LotsOfCPU",
				PodRAM:            "gigsOfRam",
			},
		},
		UpdatedAt: time.Now(),
		Status:    "UNHEALTHY",
	}

	s.Run("returns alert emails, queues and variables for the requested deployment with CeleryExecutor", func() {
		var expectedAdditional, actualAdditional orderedPieces
		qList := []map[string]interface{}{
			{
				"name":               "default",
				"max_worker_count":   130,
				"min_worker_count":   12,
				"worker_concurrency": 110,
				"worker_type":        "test-instance-type",
			},
			{
				"name":               "test-queue-1",
				"max_worker_count":   175,
				"min_worker_count":   8,
				"worker_concurrency": 150,
				"worker_type":        "test-instance-type-1",
			},
		}
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		rawExpected := map[string]interface{}{
			"alert_emails":          sourceDeployment.AlertEmails,
			"worker_queues":         qList,
			"environment_variables": getVariablesMap(sourceDeployment.DeploymentSpec.EnvironmentVariablesObjects), // API only returns values when !EnvironmentVariablesObject.isSecret
		}
		rawAdditional := getAdditional(&sourceDeployment)
		err := decodeToStruct(rawAdditional, &actualAdditional)
		s.NoError(err)
		err = decodeToStruct(rawExpected, &expectedAdditional)
		s.NoError(err)
		s.Equal(expectedAdditional, actualAdditional)
	})
	s.Run("returns alert emails, queues and variables for the requested deployment with KubernetesExecutor", func() {
		var expectedAdditional, actualAdditional orderedPieces
		sourceDeployment.DeploymentSpec.Executor = "KubernetesExecutor"
		qList := []map[string]interface{}{
			{
				"name":        "default",
				"pod_cpu":     "SmallCPU",
				"pod_ram":     "megsOfRam",
				"worker_type": "test-instance-type",
			},
			{
				"name":        "test-queue-1",
				"pod_cpu":     "LotsOfCPU",
				"pod_ram":     "gigsOfRam",
				"worker_type": "test-instance-type-1",
			},
		}
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		rawExpected := map[string]interface{}{
			"alert_emails":          sourceDeployment.AlertEmails,
			"worker_queues":         qList,
			"environment_variables": getVariablesMap(sourceDeployment.DeploymentSpec.EnvironmentVariablesObjects), // API only returns values when !EnvironmentVariablesObject.isSecret
		}
		rawAdditional := getAdditional(&sourceDeployment)
		err := decodeToStruct(rawAdditional, &actualAdditional)
		s.NoError(err)
		err = decodeToStruct(rawExpected, &expectedAdditional)
		s.NoError(err)
		s.Equal(expectedAdditional, actualAdditional)
	})
}

func (s *Suite) TestFormatPrintableDeployment() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	sourceDeployment := astro.Deployment{
		ID:          "test-deployment-id",
		Label:       "test-deployment-label",
		Description: "description",
		Workspace:   astro.Workspace{ID: "test-ws-id", Label: "test-ws"},
		ReleaseName: "great-release-name",
		AlertEmails: []string{"email1", "email2"},
		Cluster: astro.Cluster{
			ID:   "cluster-id",
			Name: "test-cluster",
			NodePools: []astro.NodePool{
				{
					ID:               "test-pool-id",
					IsDefault:        false,
					NodeInstanceType: "test-instance-type",
					CreatedAt:        time.Now(),
				},
				{
					ID:               "test-pool-id-1",
					IsDefault:        true,
					NodeInstanceType: "test-instance-type-1",
					CreatedAt:        time.Now(),
				},
			},
		},
		DagDeployEnabled: true,
		RuntimeRelease:   astro.RuntimeRelease{Version: "6.0.0", AirflowVersion: "2.4.0"},
		DeploymentSpec: astro.DeploymentSpec{
			Executor: "CeleryExecutor",
			Scheduler: astro.Scheduler{
				AU:       5,
				Replicas: 3,
			},
			Webserver: astro.Webserver{URL: "some-url"},
			EnvironmentVariablesObjects: []astro.EnvironmentVariablesObject{
				{
					Key:       "foo",
					Value:     "bar",
					IsSecret:  false,
					UpdatedAt: "NOW",
				},
				{
					Key:       "bar",
					Value:     "baz",
					IsSecret:  true,
					UpdatedAt: "NOW+1",
				},
			},
		},
		WorkerQueues: []astro.WorkerQueue{
			{
				ID:                "test-wq-id",
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    130,
				MinWorkerCount:    12,
				WorkerConcurrency: 110,
				NodePoolID:        "test-pool-id",
				PodCPU:            "smallCPU",
				PodRAM:            "megsOfRam",
			},
			{
				ID:                "test-wq-id-1",
				Name:              "test-queue-1",
				IsDefault:         false,
				MaxWorkerCount:    175,
				MinWorkerCount:    8,
				WorkerConcurrency: 150,
				NodePoolID:        "test-pool-id-1",
				PodCPU:            "LotsOfCPU",
				PodRAM:            "gigsOfRam",
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Status:    "UNHEALTHY",
	}
	var expectedPrintableDeployment []byte

	s.Run("returns a yaml formatted printable deployment", func() {
		info, _ := getDeploymentInfo(&sourceDeployment)
		config := getDeploymentConfig(&sourceDeployment)
		additional := getAdditional(&sourceDeployment)

		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
			},
		}
		expectedDeployment := `deployment:
    environment_variables:
        - is_secret: false
          key: foo
          updated_at: NOW
          value: bar
        - is_secret: true
          key: bar
          updated_at: NOW+1
          value: baz
    configuration:
        name: test-deployment-label
        description: description
        runtime_version: 6.0.0
        scheduler_au: 5
        scheduler_count: 3
        cluster_id: cluster-id
    worker_queues:
        - name: default
          id: test-wq-id
          is_default: true
          max_worker_count: 130
          min_worker_count: 12
          worker_concurrency: 110
          node_pool_id: test-pool-id
        - name: test-queue-1
          id: test-wq-id-1
          is_default: false
          max_worker_count: 175
          min_worker_count: 8
          worker_concurrency: 150
          node_pool_id: test-pool-id-1
    metadata:
        deployment_id: test-deployment-id
        workspace_id: test-ws-id
        cluster_id: cluster-id
        release_name: great-release-name
        airflow_version: 2.4.0
        dag_deploy_enabled: true
        status: UNHEALTHY
        created_at: 2022-11-17T13:25:55.275697-08:00
        updated_at: 2022-11-17T13:25:55.275697-08:00
        deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/analytics
        webserver_url: some-url
    alert_emails:
        - email1
        - email2
`
		var orderedAndTaggedDeployment, unorderedDeployment FormattedDeployment
		actualPrintableDeployment, err := formatPrintableDeployment("", false, printableDeployment)
		s.NoError(err)
		// testing we get valid yaml
		err = yaml.Unmarshal(actualPrintableDeployment, &orderedAndTaggedDeployment)
		s.NoError(err)
		// update time and create time are not equal here so can not do equality check
		s.NotEqual(expectedDeployment, string(actualPrintableDeployment), "tag and order should match")

		unordered, err := yaml.Marshal(printableDeployment)
		s.NoError(err)
		err = yaml.Unmarshal(unordered, &unorderedDeployment)
		s.NoError(err)
		// testing the structs are equal regardless of order
		s.Equal(orderedAndTaggedDeployment, unorderedDeployment, "structs should match")
		// testing the order is not equal
		s.NotEqual(string(unordered), string(actualPrintableDeployment), "order should not match")
	})
	s.Run("returns a yaml formatted template deployment", func() {
		info, _ := getDeploymentInfo(&sourceDeployment)
		config := getDeploymentConfig(&sourceDeployment)
		additional := getAdditional(&sourceDeployment)

		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
			},
		}
		expectedDeployment := `deployment:
    environment_variables:
        - is_secret: false
          key: foo
          updated_at: NOW
          value: bar
    configuration:
        name: ""
        description: description
        runtime_version: 6.0.0
        dag_deploy_enabled: true
        executor: CeleryExecutor
        scheduler_au: 5
        scheduler_count: 3
        cluster_name: test-cluster
        workspace_name: test-ws
    worker_queues:
        - name: default
          max_worker_count: 130
          min_worker_count: 12
          worker_concurrency: 110
          worker_type: test-instance-type
        - name: test-queue-1
          max_worker_count: 175
          min_worker_count: 8
          worker_concurrency: 150
          worker_type: test-instance-type-1
    alert_emails:
        - email1
        - email2
`
		var orderedAndTaggedDeployment FormattedDeployment
		actualPrintableDeployment, err := formatPrintableDeployment("", true, printableDeployment)
		s.NoError(err)
		// testing we get valid yaml
		err = yaml.Unmarshal(actualPrintableDeployment, &orderedAndTaggedDeployment)
		s.NoError(err)
		s.Equal(expectedDeployment, string(actualPrintableDeployment), "tag and order should match")
	})
	s.Run("returns a json formatted printable deployment", func() {
		info, _ := getDeploymentInfo(&sourceDeployment)
		config := getDeploymentConfig(&sourceDeployment)
		additional := getAdditional(&sourceDeployment)
		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
			},
		}
		expectedDeployment := `{
    "deployment": {
        "environment_variables": [
            {
                "is_secret": false,
                "key": "foo",
                "updated_at": "NOW",
                "value": "bar"
            },
            {
                "is_secret": true,
                "key": "bar",
                "updated_at": "NOW+1",
                "value": "baz"
            }
        ],
        "configuration": {
            "name": "test-deployment-label",
            "description": "description",
            "runtime_version": "6.0.0",
            "scheduler_au": 5,
            "scheduler_count": 3,
            "cluster_id": "cluster-id"
        },
        "worker_queues": [
            {
                "name": "default",
                "id": "test-wq-id",
                "is_default": true,
                "max_worker_count": 130,
                "min_worker_count": 12,
                "worker_concurrency": 110,
                "node_pool_id": "test-pool-id"
            },
            {
                "name": "test-queue-1",
                "id": "test-wq-id-1",
                "is_default": false,
                "max_worker_count": 175,
                "min_worker_count": 8,
                "worker_concurrency": 150,
                "node_pool_id": "test-pool-id-1"
            }
        ],
        "metadata": {
            "deployment_id": "test-deployment-id",
            "workspace_id": "test-ws-id",
            "cluster_id": "cluster-id",
            "release_name": "great-release-name",
            "airflow_version": "2.4.0",
            "dag_deploy_enabled": true,
            "status": "UNHEALTHY",
            "created_at": "2022-11-17T12:26:45.362983-08:00",
            "updated_at": "2022-11-17T12:26:45.362983-08:00",
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/analytics",
            "webserver_url": "some-url"
        },
        "alert_emails": [
            "email1",
            "email2"
        ]
    }
}`
		var orderedAndTaggedDeployment, unorderedDeployment FormattedDeployment
		actualPrintableDeployment, err := formatPrintableDeployment("json", false, printableDeployment)
		s.NoError(err)
		// testing we get valid json
		err = json.Unmarshal(actualPrintableDeployment, &orderedAndTaggedDeployment)
		s.NoError(err)
		// update time and create time are not equal here so can not do equality check
		s.NotEqual(expectedDeployment, string(actualPrintableDeployment), "tag and order should match")

		unordered, err := json.MarshalIndent(printableDeployment, "", "    ")
		s.NoError(err)
		err = json.Unmarshal(unordered, &unorderedDeployment)
		s.NoError(err)
		// testing the structs are equal regardless of order
		s.Equal(orderedAndTaggedDeployment, unorderedDeployment, "structs should match")
		// testing the order is not equal
		s.NotEqual(string(unordered), string(actualPrintableDeployment), "order should not match")
	})
	s.Run("returns a json formatted template deployment", func() {
		sourceDeployment.DeploymentSpec.Executor = "KubernetesExecutor"
		info, _ := getDeploymentInfo(&sourceDeployment)
		config := getDeploymentConfig(&sourceDeployment)
		additional := getAdditional(&sourceDeployment)
		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
			},
		}
		expectedDeployment := `{
    "deployment": {
        "environment_variables": [
            {
                "is_secret": false,
                "key": "foo",
                "updated_at": "NOW",
                "value": "bar"
            }
        ],
        "configuration": {
            "name": "",
            "description": "description",
            "runtime_version": "6.0.0",
            "dag_deploy_enabled": true,
            "executor": "KubernetesExecutor",
            "scheduler_au": 5,
            "scheduler_count": 3,
            "cluster_name": "test-cluster",
            "workspace_name": "test-ws"
        },
        "worker_queues": [
            {
                "name": "default",
                "worker_type": "test-instance-type",
                "pod_cpu": "smallCPU",
                "pod_ram": "megsOfRam"
            },
            {
                "name": "test-queue-1",
                "worker_type": "test-instance-type-1",
                "pod_cpu": "LotsOfCPU",
                "pod_ram": "gigsOfRam"
            }
        ],
        "alert_emails": [
            "email1",
            "email2"
        ]
    }
}`
		var orderedAndTaggedDeployment FormattedDeployment
		actualPrintableDeployment, err := formatPrintableDeployment("json", true, printableDeployment)
		s.NoError(err)
		// testing we get valid json
		err = json.Unmarshal(actualPrintableDeployment, &orderedAndTaggedDeployment)
		s.NoError(err)
		s.Equal(expectedDeployment, string(actualPrintableDeployment), "tag and order should match")
	})
	s.Run("returns an error if decoding to struct fails", func() {
		originalDecode := decodeToStruct
		decodeToStruct = errorReturningDecode
		defer restoreDecode(originalDecode)
		info, _ := getDeploymentInfo(&sourceDeployment)
		config := getDeploymentConfig(&sourceDeployment)
		additional := getAdditional(&sourceDeployment)
		expectedPrintableDeployment = []byte{}
		actualPrintableDeployment, err := formatPrintableDeployment("", false, getPrintableDeployment(info, config, additional))
		s.ErrorIs(err, errMarshal)
		s.Contains(string(actualPrintableDeployment), string(expectedPrintableDeployment))
	})
	s.Run("returns an error if marshaling yaml fails", func() {
		originalMarshal := yamlMarshal
		yamlMarshal = errReturningYAMLMarshal
		defer restoreYAMLMarshal(originalMarshal)
		info, _ := getDeploymentInfo(&sourceDeployment)
		config := getDeploymentConfig(&sourceDeployment)
		additional := getAdditional(&sourceDeployment)
		expectedPrintableDeployment = []byte{}
		actualPrintableDeployment, err := formatPrintableDeployment("", false, getPrintableDeployment(info, config, additional))
		s.ErrorIs(err, errMarshal)
		s.Contains(string(actualPrintableDeployment), string(expectedPrintableDeployment))
	})
	s.Run("returns an error if marshaling json fails", func() {
		originalMarshal := jsonMarshal
		jsonMarshal = errReturningJSONMarshal
		defer restoreJSONMarshal(originalMarshal)
		info, _ := getDeploymentInfo(&sourceDeployment)
		config := getDeploymentConfig(&sourceDeployment)
		additional := getAdditional(&sourceDeployment)
		expectedPrintableDeployment = []byte{}
		actualPrintableDeployment, err := formatPrintableDeployment("json", false, getPrintableDeployment(info, config, additional))
		s.ErrorIs(err, errMarshal)
		s.Contains(string(actualPrintableDeployment), string(expectedPrintableDeployment))
	})
}

func (s *Suite) TestGetSpecificField() {
	sourceDeployment := astro.Deployment{
		ID:          "test-deployment-id",
		Label:       "test-deployment-label",
		Description: "description",
		Workspace:   astro.Workspace{ID: "test-ws-id", Label: "test-workspace"},
		ReleaseName: "great-release-name",
		AlertEmails: []string{"email1", "email2"},
		Cluster: astro.Cluster{
			ID:   "cluster-id",
			Name: "test-cluster",
			NodePools: []astro.NodePool{
				{
					ID:               "test-pool-id",
					IsDefault:        false,
					NodeInstanceType: "test-instance-type",
					CreatedAt:        time.Now(),
				},
				{
					ID:               "test-pool-id-1",
					IsDefault:        true,
					NodeInstanceType: "test-instance-type-1",
					CreatedAt:        time.Now(),
				},
			},
		},
		RuntimeRelease: astro.RuntimeRelease{Version: "6.0.0", AirflowVersion: "2.4.0"},
		DeploymentSpec: astro.DeploymentSpec{
			Executor: "CeleryExecutor",
			Scheduler: astro.Scheduler{
				AU:       5,
				Replicas: 3,
			},
			Webserver: astro.Webserver{URL: "some-url"},
			EnvironmentVariablesObjects: []astro.EnvironmentVariablesObject{
				{
					Key:       "foo",
					Value:     "bar",
					IsSecret:  false,
					UpdatedAt: "NOW",
				},
				{
					Key:       "bar",
					Value:     "baz",
					IsSecret:  true,
					UpdatedAt: "NOW+1",
				},
			},
		},
		WorkerQueues: []astro.WorkerQueue{
			{
				ID:                "test-wq-id",
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    130,
				MinWorkerCount:    12,
				WorkerConcurrency: 110,
				NodePoolID:        "test-pool-id",
				PodCPU:            "SmallCPU",
				PodRAM:            "megsOfRam",
			},
			{
				ID:                "test-wq-id-1",
				Name:              "test-queue-1",
				IsDefault:         false,
				MaxWorkerCount:    175,
				MinWorkerCount:    8,
				WorkerConcurrency: 150,
				NodePoolID:        "test-pool-id-1",
				PodCPU:            "LotsOfCPU",
				PodRAM:            "gigsOfRam",
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Status:    "UNHEALTHY",
	}
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	info, _ := getDeploymentInfo(&sourceDeployment)
	config := getDeploymentConfig(&sourceDeployment)
	additional := getAdditional(&sourceDeployment)
	s.Run("returns a value if key is found in deployment.metadata", func() {
		requestedField := "metadata.workspace_id"
		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
			},
		}
		actual, err := getSpecificField(printableDeployment, requestedField)
		s.NoError(err)
		s.Equal(sourceDeployment.Workspace.ID, actual)
	})
	s.Run("returns a value if key is found in deployment.configuration", func() {
		requestedField := "configuration.scheduler_count"
		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
			},
		}
		actual, err := getSpecificField(printableDeployment, requestedField)
		s.NoError(err)
		s.Equal(sourceDeployment.DeploymentSpec.Scheduler.Replicas, actual)
	})
	s.Run("returns a value if key is alert_emails", func() {
		requestedField := "alert_emails"
		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
			},
		}
		actual, err := getSpecificField(printableDeployment, requestedField)
		s.NoError(err)
		s.Equal(sourceDeployment.AlertEmails, actual)
	})
	s.Run("returns a value if key is environment_variables", func() {
		requestedField := "environment_variables"
		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
			},
		}
		actual, err := getSpecificField(printableDeployment, requestedField)
		s.NoError(err)
		s.Equal(getVariablesMap(sourceDeployment.DeploymentSpec.EnvironmentVariablesObjects), actual)
	})
	s.Run("returns a value if key is worker_queues", func() {
		requestedField := "worker_queues"
		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
			},
		}
		actual, err := getSpecificField(printableDeployment, requestedField)
		s.NoError(err)
		s.Equal(getQMap(sourceDeployment.WorkerQueues, sourceDeployment.Cluster.NodePools, sourceDeployment.DeploymentSpec.Executor), actual)
	})
	s.Run("returns a value if key is metadata", func() {
		requestedField := "metadata"
		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
			},
		}
		actual, err := getSpecificField(printableDeployment, requestedField)
		s.NoError(err)
		s.Equal(info, actual)
	})
	s.Run("returns value regardless of upper or lower case key", func() {
		requestedField := "Configuration.Cluster_NAME"
		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
			},
		}
		actual, err := getSpecificField(printableDeployment, requestedField)
		s.NoError(err)
		s.Equal(sourceDeployment.Cluster.Name, actual)
	})
	s.Run("returns error if no value is found", func() {
		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["astronomer_variables"],
			},
		}
		requestedField := "does-not-exist"
		actual, err := getSpecificField(printableDeployment, requestedField)
		s.ErrorContains(err, "requested key "+requestedField+" not found in deployment")
		s.Equal(nil, actual)
	})
	s.Run("returns error if incorrect field is requested", func() {
		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["astronomer_variables"],
			},
		}
		requestedField := "configuration.does-not-exist"
		actual, err := getSpecificField(printableDeployment, requestedField)
		s.ErrorIs(err, errKeyNotFound)
		s.Equal(nil, actual)
	})
}

func (s *Suite) TestGetWorkerTypeFromNodePoolID() {
	var (
		expectedWorkerType, poolID, actualWorkerType string
		existingPools                                []astro.NodePool
	)
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	expectedWorkerType = "worker-1"
	poolID = "test-pool-id"
	existingPools = []astro.NodePool{
		{
			ID:               "test-pool-id",
			IsDefault:        false,
			NodeInstanceType: "worker-1",
		},
		{
			ID:               "test-pool-id",
			IsDefault:        false,
			NodeInstanceType: "worker-2",
		},
	}
	s.Run("returns a worker type from cluster for pool with matching nodepool id", func() {
		actualWorkerType = getWorkerTypeFromNodePoolID(poolID, existingPools)
		s.Equal(expectedWorkerType, actualWorkerType)
	})
	s.Run("returns an empty worker type if no pool with matching node pool id exists in the cluster", func() {
		poolID = "test-pool-id-1"
		actualWorkerType = getWorkerTypeFromNodePoolID(poolID, existingPools)
		s.Equal("", actualWorkerType)
	})
}

func (s *Suite) TestGetTemplate() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	sourceDeployment := astro.Deployment{
		ID:          "test-deployment-id",
		Label:       "test-deployment-label",
		Description: "description",
		Workspace:   astro.Workspace{ID: "test-ws-id"},
		ReleaseName: "great-release-name",
		AlertEmails: []string{"email1", "email2"},
		Cluster: astro.Cluster{
			ID: "cluster-id",
			NodePools: []astro.NodePool{
				{
					ID:               "test-pool-id",
					IsDefault:        false,
					NodeInstanceType: "test-instance-type",
					CreatedAt:        time.Now(),
				},
				{
					ID:               "test-pool-id-1",
					IsDefault:        true,
					NodeInstanceType: "test-instance-type-1",
					CreatedAt:        time.Now(),
				},
			},
		},
		DagDeployEnabled: true,
		RuntimeRelease:   astro.RuntimeRelease{Version: "6.0.0", AirflowVersion: "2.4.0"},
		DeploymentSpec: astro.DeploymentSpec{
			Executor: "CeleryExecutor",
			Scheduler: astro.Scheduler{
				AU:       5,
				Replicas: 3,
			},
			Webserver: astro.Webserver{URL: "some-url"},
			EnvironmentVariablesObjects: []astro.EnvironmentVariablesObject{
				{
					Key:       "foo",
					Value:     "bar",
					IsSecret:  false,
					UpdatedAt: "NOW",
				},
				{
					Key:       "bar",
					Value:     "baz",
					IsSecret:  true,
					UpdatedAt: "NOW+1",
				},
			},
		},
		WorkerQueues: []astro.WorkerQueue{
			{
				ID:                "test-wq-id",
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    130,
				MinWorkerCount:    12,
				WorkerConcurrency: 110,
				NodePoolID:        "test-pool-id",
			},
			{
				ID:                "test-wq-id-1",
				Name:              "test-queue-1",
				IsDefault:         false,
				MaxWorkerCount:    175,
				MinWorkerCount:    8,
				WorkerConcurrency: 150,
				NodePoolID:        "test-pool-id-1",
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Status:    "UNHEALTHY",
	}
	info, _ := getDeploymentInfo(&sourceDeployment)
	config := getDeploymentConfig(&sourceDeployment)
	additional := getAdditional(&sourceDeployment)

	s.Run("returns a formatted template", func() {
		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
			},
		}
		var decoded, expected FormattedDeployment
		err := decodeToStruct(printableDeployment, &decoded)
		s.NoError(err)
		err = decodeToStruct(printableDeployment, &expected)
		s.NoError(err)
		expected.Deployment.Configuration.Name = ""
		expected.Deployment.Metadata = nil
		newEnvVars := []EnvironmentVariable{}
		for i := range expected.Deployment.EnvVars {
			if !expected.Deployment.EnvVars[i].IsSecret {
				newEnvVars = append(newEnvVars, expected.Deployment.EnvVars[i])
			}
		}
		expected.Deployment.EnvVars = newEnvVars
		for i := range expected.Deployment.EnvVars {
			expected.Deployment.EnvVars[i].UpdatedAt = "NOW"
		}

		actual := getTemplate(&decoded)
		s.Equal(expected, actual)
	})
	s.Run("returns a template without env vars if they are empty", func() {
		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":      info,
				"configuration": config,
				"alert_emails":  additional["alert_emails"],
				"worker_queues": additional["worker_queues"],
			},
		}
		var decoded, expected FormattedDeployment
		err := decodeToStruct(printableDeployment, &decoded)
		s.NoError(err)
		err = decodeToStruct(printableDeployment, &expected)
		s.NoError(err)
		err = decodeToStruct(printableDeployment, &decoded)
		s.NoError(err)
		err = decodeToStruct(printableDeployment, &expected)
		s.NoError(err)
		expected.Deployment.Configuration.Name = ""
		expected.Deployment.Metadata = nil
		expected.Deployment.EnvVars = nil
		newEnvVars := []EnvironmentVariable{}
		for i := range expected.Deployment.EnvVars {
			if !expected.Deployment.EnvVars[i].IsSecret {
				newEnvVars = append(newEnvVars, expected.Deployment.EnvVars[i])
			}
		}
		for i := range expected.Deployment.EnvVars {
			expected.Deployment.EnvVars[i].UpdatedAt = "NOW"
		}
		expected.Deployment.EnvVars = newEnvVars
		actual := getTemplate(&decoded)
		s.Equal(expected, actual)
	})
	s.Run("returns a template without alert emails if they are empty", func() {
		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
			},
		}
		var decoded, expected FormattedDeployment
		err := decodeToStruct(printableDeployment, &decoded)
		s.NoError(err)
		err = decodeToStruct(printableDeployment, &expected)
		s.NoError(err)
		expected.Deployment.Configuration.Name = ""
		expected.Deployment.Metadata = nil
		expected.Deployment.AlertEmails = nil
		newEnvVars := []EnvironmentVariable{}
		for i := range expected.Deployment.EnvVars {
			if !expected.Deployment.EnvVars[i].IsSecret {
				newEnvVars = append(newEnvVars, expected.Deployment.EnvVars[i])
			}
		}
		for i := range expected.Deployment.EnvVars {
			expected.Deployment.EnvVars[i].UpdatedAt = ""
		}
		expected.Deployment.EnvVars = newEnvVars
		actual := getTemplate(&decoded)
		s.Equal(expected, actual)
	})
}
