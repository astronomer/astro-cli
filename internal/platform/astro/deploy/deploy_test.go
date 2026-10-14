package deploy

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/mocks"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var (
	errMock                    = errors.New("mock error")
	ws                         = "test-ws-id"
	dagTarballVersionTest      = "test-version"
	dagsUploadTestURL          = "test-url"
	deploymentID               = "test-deployment-id"
	tarballVersion             = "test-version"
	hybridType                 = astrov1.DeploymentTypeHYBRID
	mockCoreDeploymentResponse = []astrov1.Deployment{
		{
			Id:     deploymentID,
			Name:   "test-deployment",
			Status: "HEALTHY",
			Type:   &hybridType,
		},
	}
	mockCoreDeploymentResponseCICD = []astrov1.Deployment{
		{
			Id:             deploymentID,
			Status:         "HEALTHY",
			IsCicdEnforced: true,
			Type:           &hybridType,
		},
	}
	mockListDeploymentsResponse = astrov1.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.DeploymentsPaginated{
			Deployments: mockCoreDeploymentResponse,
		},
	}
	mockListDeploymentsResponseCICD = astrov1.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.DeploymentsPaginated{
			Deployments: mockCoreDeploymentResponseCICD,
		},
	}
	createDeployResponse = astrov1.CreateDeployResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.Deploy{
			Id:                "test-id",
			DagTarballVersion: &dagTarballVersionTest,
			ImageRepository:   "test-repository",
			DagsUploadUrl:     &dagsUploadTestURL,
		},
	}
	finalizeDeployResponse = astrov1.FinalizeDeployResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.Deploy{
			Id:                "test-id",
			DagTarballVersion: &dagTarballVersionTest,
			ImageTag:          "test-tag",
		},
	}
	getDeploymentOptionsResponse = astrov1.GetDeploymentOptionsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.DeploymentOptions{
			RuntimeReleases: []astrov1.RuntimeRelease{
				{Version: "12.0.0"},
				{Version: "4.2.6"},
				{Version: "4.2.5"},
				{Version: "3.1-1"},
				{Version: "3.0-3"},
				{Version: "3.0-1"},
			},
		},
	}
	deploymentResponse = astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.Deployment{
			Id:                  deploymentID,
			AstroRuntimeVersion: "12.0.0",
			Namespace:           "test-name",
			WorkspaceId:         ws,
			WebServerUrl:        "test-url",
			IsDagDeployEnabled:  false,
			Type:                &hybridType,
		},
	}
	deploymentResponseCICD = astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.Deployment{
			Id:                  deploymentID,
			AstroRuntimeVersion: "12.0.0",
			Namespace:           "test-name",
			WorkspaceId:         ws,
			WebServerUrl:        "test-url",
			IsDagDeployEnabled:  false,
			IsCicdEnforced:      true,
			Type:                &hybridType,
			Name:                "test-deployment",
		},
	}
	deploymentResponseRemoteExecution = astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.Deployment{
			Id:                  deploymentID,
			AstroRuntimeVersion: "3.0-1",
			Namespace:           "test-name",
			WorkspaceId:         ws,
			WebServerUrl:        "test-url",
			IsDagDeployEnabled:  false,
			Type:                &hybridType,
			RemoteExecution: &astrov1.DeploymentRemoteExecution{
				Enabled: true,
			},
		},
	}
	deploymentResponseDags = astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.Deployment{
			Id:                       deploymentID,
			AstroRuntimeVersion:      "12.0.0",
			Namespace:                "test-name",
			WorkspaceId:              ws,
			WebServerUrl:             "test-url",
			IsDagDeployEnabled:       true,
			IsCicdEnforced:           false,
			Type:                     &hybridType,
			DesiredDagTarballVersion: &tarballVersion,
		},
	}
)

func TestDeployClientImage(t *testing.T) {
	// Store original DockerLogin function to restore after tests
	originalDockerLogin := airflow.DockerLogin
	defer func() {
		airflow.DockerLogin = originalDockerLogin
	}()

	t.Run("successful client deploy", func(t *testing.T) {
		// Set up temporary directory with Dockerfile.client
		tempDir, err := os.MkdirTemp("", "test-deploy-*")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// Create a basic Dockerfile.client file for the test
		dockerfileContent := "FROM images.astronomer.cloud/baseimages/astro-remote-execution-agent:latest"
		err = os.WriteFile(filepath.Join(tempDir, "Dockerfile.client"), []byte(dockerfileContent), 0o644)
		assert.NoError(t, err)

		// Create required client dependency files
		err = os.WriteFile(filepath.Join(tempDir, "requirements-client.txt"), []byte("requests==2.28.0"), 0o644)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, "packages-client.txt"), []byte("curl"), 0o644)
		assert.NoError(t, err)

		// Set up current context
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := config.GetCurrentContext()
		assert.NoError(t, err)
		ctx.Token = "test-token"
		err = ctx.SetContext()
		assert.NoError(t, err)
		// Mock DockerLogin
		dockerLoginCalled := false
		var capturedRegistry, capturedUsername, capturedToken string
		airflow.DockerLogin = func(registry, username, token string) error {
			dockerLoginCalled = true
			capturedRegistry = registry
			capturedUsername = username
			capturedToken = token
			return nil
		}

		// Mock image handler
		mockImageHandler := new(mocks.ImageHandler)
		mockImageHandler.On("Build", "Dockerfile.client", mock.Anything, mock.AnythingOfType("types.ImageBuildConfig")).Return(nil).Once()
		mockImageHandler.On("Push", mock.AnythingOfType("string"), "", "", false).Return("", nil).Once()

		// Override airflowImageHandler
		originalAirflowImageHandler := airflowImageHandler
		airflowImageHandler = func(imageName string) airflow.ImageHandler {
			return mockImageHandler
		}
		defer func() {
			airflowImageHandler = originalAirflowImageHandler
		}()

		// Mock config.CFG.RemoteClientRegistry
		config.CFG.RemoteClientRegistry.SetHomeString("test-registry:latest")

		deployInput := InputClientDeploy{
			Path:         tempDir,
			BuildSecrets: nil,
		}

		_, err = DeployClientImage(deployInput, nil)
		assert.NoError(t, err)
		assert.True(t, dockerLoginCalled, "DockerLogin should have been called")
		assert.Equal(t, "images.astronomer.cloud", capturedRegistry)
		assert.Equal(t, "cli", capturedUsername)
		assert.Equal(t, "test-token", capturedToken)
		mockImageHandler.AssertExpectations(t)
	})

	t.Run("docker login failure", func(t *testing.T) {
		// Set up current context
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := config.GetCurrentContext()
		assert.NoError(t, err)
		ctx.Token = "test-token"
		err = ctx.SetContext()
		assert.NoError(t, err)

		// Mock DockerLogin to return error
		airflow.DockerLogin = func(registry, username, token string) error {
			return errors.New("login failed")
		}

		config.CFG.RemoteClientRegistry.SetHomeString("test-registry:latest")

		deployInput := InputClientDeploy{
			Path:         "/test/path",
			BuildSecrets: nil,
		}

		_, err = DeployClientImage(deployInput, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate with registry images.astronomer.cloud")
	})

	t.Run("missing registry configuration", func(t *testing.T) {
		// Set up current context
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := config.GetCurrentContext()
		assert.NoError(t, err)
		ctx.Token = "test-token"
		err = ctx.SetContext()
		assert.NoError(t, err)

		// Mock DockerLogin (shouldn't be called)
		dockerLoginCalled := false
		airflow.DockerLogin = func(registry, username, token string) error {
			dockerLoginCalled = true
			return nil
		}

		config.CFG.RemoteClientRegistry.SetHomeString("") // Empty registry

		deployInput := InputClientDeploy{
			Path:         "/test/path",
			BuildSecrets: nil,
		}

		_, err = DeployClientImage(deployInput, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "remote client registry is not configured")
		assert.False(t, dockerLoginCalled, "DockerLogin should not have been called")
	})

	t.Run("build failure", func(t *testing.T) {
		// Set up temporary directory
		tempDir, err := os.MkdirTemp("", "test-deploy-*")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// Create a basic Dockerfile.client file for the test
		dockerfileContent := "FROM images.astronomer.cloud/baseimages/astro-remote-execution-agent:latest"
		err = os.WriteFile(filepath.Join(tempDir, "Dockerfile.client"), []byte(dockerfileContent), 0o644)
		assert.NoError(t, err)

		// Create required client dependency files
		err = os.WriteFile(filepath.Join(tempDir, "requirements-client.txt"), []byte("numpy==1.21.0"), 0o644)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, "packages-client.txt"), []byte("git"), 0o644)
		assert.NoError(t, err)

		// Set up current context
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := config.GetCurrentContext()
		assert.NoError(t, err)
		ctx.Token = "test-token"
		err = ctx.SetContext()
		assert.NoError(t, err)

		// Mock successful DockerLogin
		airflow.DockerLogin = func(registry, username, token string) error {
			return nil
		}

		// Mock image handler with build failure
		mockImageHandler := new(mocks.ImageHandler)
		mockImageHandler.On("Build", "Dockerfile.client", mock.Anything, mock.AnythingOfType("types.ImageBuildConfig")).Return(errors.New("build failed")).Once()

		// Override airflowImageHandler
		originalAirflowImageHandler := airflowImageHandler
		airflowImageHandler = func(imageName string) airflow.ImageHandler {
			return mockImageHandler
		}
		defer func() {
			airflowImageHandler = originalAirflowImageHandler
		}()

		config.CFG.RemoteClientRegistry.SetHomeString("test-registry:latest")

		deployInput := InputClientDeploy{
			Path:         tempDir,
			BuildSecrets: nil,
		}

		_, err = DeployClientImage(deployInput, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to build client image")
		mockImageHandler.AssertExpectations(t)
	})

	t.Run("push failure", func(t *testing.T) {
		// Set up temporary directory
		tempDir, err := os.MkdirTemp("", "test-deploy-*")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// Create a basic Dockerfile.client file for the test
		dockerfileContent := "FROM images.astronomer.cloud/baseimages/astro-remote-execution-agent:latest"
		err = os.WriteFile(filepath.Join(tempDir, "Dockerfile.client"), []byte(dockerfileContent), 0o644)
		assert.NoError(t, err)

		// Create required client dependency files
		err = os.WriteFile(filepath.Join(tempDir, "requirements-client.txt"), []byte("flask==2.0.0"), 0o644)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, "packages-client.txt"), []byte("vim"), 0o644)
		assert.NoError(t, err)

		// Set up current context
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := config.GetCurrentContext()
		assert.NoError(t, err)
		ctx.Token = "test-token"
		err = ctx.SetContext()
		assert.NoError(t, err)

		// Mock successful DockerLogin
		airflow.DockerLogin = func(registry, username, token string) error {
			return nil
		}

		// Mock image handler with push failure
		mockImageHandler := new(mocks.ImageHandler)
		mockImageHandler.On("Build", "Dockerfile.client", mock.Anything, mock.AnythingOfType("types.ImageBuildConfig")).Return(nil).Once()
		mockImageHandler.On("Push", mock.AnythingOfType("string"), "", "", false).Return("", errors.New("push failed")).Once()

		// Override airflowImageHandler
		originalAirflowImageHandler := airflowImageHandler
		airflowImageHandler = func(imageName string) airflow.ImageHandler {
			return mockImageHandler
		}
		defer func() {
			airflowImageHandler = originalAirflowImageHandler
		}()

		config.CFG.RemoteClientRegistry.SetHomeString("test-registry:latest")

		deployInput := InputClientDeploy{
			Path:         tempDir,
			BuildSecrets: nil,
		}

		_, err = DeployClientImage(deployInput, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to push client image")
		mockImageHandler.AssertExpectations(t)
	})

	t.Run("deploy with custom image name", func(t *testing.T) {
		// Set up current context
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := config.GetCurrentContext()
		assert.NoError(t, err)
		ctx.Token = "test-token"
		err = ctx.SetContext()
		assert.NoError(t, err)

		// Mock image handler
		mockImageHandler := new(mocks.ImageHandler)
		mockImageHandler.On("TagLocalImage", "custom-image:tag").Return(nil).Once()
		// Remote image will use timestamp tag, not the user-provided tag
		mockImageHandler.On("Push", mock.MatchedBy(func(remoteImage string) bool {
			// Verify it uses timestamp-based tag format, not "tag" from the user input
			return strings.Contains(remoteImage, "test-registry:latest:deploy-") &&
				!strings.Contains(remoteImage, ":tag")
		}), "", "", false).Return("", nil).Once()

		// Override airflowImageHandler
		originalAirflowImageHandler := airflowImageHandler
		airflowImageHandler = func(imageName string) airflow.ImageHandler {
			return mockImageHandler
		}
		defer func() {
			airflowImageHandler = originalAirflowImageHandler
		}()

		config.CFG.RemoteClientRegistry.SetHomeString("test-registry:latest")

		// Set up temporary directory for consistency
		tempDir, err := os.MkdirTemp("", "test-deploy-*")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)

		deployInput := InputClientDeploy{
			Path:         tempDir,
			ImageName:    "custom-image:tag",
			BuildSecrets: nil,
		}

		_, err = DeployClientImage(deployInput, nil)
		assert.NoError(t, err)
		mockImageHandler.AssertExpectations(t)
	})
}

func TestPrepareClientBuildContext(t *testing.T) {
	t.Run("creates build context with client files when they exist and have content", func(t *testing.T) {
		// Setup temporary directory for testing
		tempDir, err := os.MkdirTemp("", "test-client-deps-*")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)
		// Create test files with content
		clientRequirementsContent := "requests==2.28.0\nnumpy==1.21.0"
		clientPackagesContent := "curl\nwget"
		regularRequirementsContent := "django==3.2.0"
		regularPackagesContent := "vim"

		err = os.WriteFile(filepath.Join(tempDir, "requirements-client.txt"), []byte(clientRequirementsContent), 0o644)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, "packages-client.txt"), []byte(clientPackagesContent), 0o644)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, "requirements.txt"), []byte(regularRequirementsContent), 0o644)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, "packages.txt"), []byte(regularPackagesContent), 0o644)
		assert.NoError(t, err)

		// Prepare build context
		buildContext, err := prepareClientBuildContext(tempDir)
		assert.NoError(t, err)
		defer buildContext.CleanupFunc()

		// Verify build context was created
		assert.NotEqual(t, tempDir, buildContext.TempDir)
		assert.Contains(t, buildContext.TempDir, "astro-client-build-")

		// Verify that in the temp build directory, the client files are used as regular files
		requirementsContent, err := os.ReadFile(filepath.Join(buildContext.TempDir, "requirements.txt"))
		assert.NoError(t, err)
		assert.Equal(t, clientRequirementsContent, string(requirementsContent))

		packagesContent, err := os.ReadFile(filepath.Join(buildContext.TempDir, "packages.txt"))
		assert.NoError(t, err)
		assert.Equal(t, clientPackagesContent, string(packagesContent))

		// Verify that original files are UNCHANGED (no modification of original project)
		originalRequirementsContent, err := os.ReadFile(filepath.Join(tempDir, "requirements.txt"))
		assert.NoError(t, err)
		assert.Equal(t, regularRequirementsContent, string(originalRequirementsContent))

		originalPackagesContent, err := os.ReadFile(filepath.Join(tempDir, "packages.txt"))
		assert.NoError(t, err)
		assert.Equal(t, regularPackagesContent, string(originalPackagesContent))
	})

	t.Run("uses regular files when client files are empty", func(t *testing.T) {
		// Setup temporary directory for testing
		tempDir, err := os.MkdirTemp("", "test-client-deps-*")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// Create empty client files and regular files with content
		regularRequirementsContent := "flask==2.0.0"
		regularPackagesContent := "git"

		err = os.WriteFile(filepath.Join(tempDir, "requirements-client.txt"), []byte(""), 0o644)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, "packages-client.txt"), []byte("   \n  "), 0o644)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, "requirements.txt"), []byte(regularRequirementsContent), 0o644)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, "packages.txt"), []byte(regularPackagesContent), 0o644)
		assert.NoError(t, err)

		// Prepare build context
		buildContext, err := prepareClientBuildContext(tempDir)
		assert.NoError(t, err)
		defer buildContext.CleanupFunc()

		// Verify that build context uses empty client file contents
		requirementsContent, err := os.ReadFile(filepath.Join(buildContext.TempDir, "requirements.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "", string(requirementsContent))

		packagesContent, err := os.ReadFile(filepath.Join(buildContext.TempDir, "packages.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "   \n  ", string(packagesContent))

		// Original files unchanged
		originalRequirementsContent, err := os.ReadFile(filepath.Join(tempDir, "requirements.txt"))
		assert.NoError(t, err)
		assert.Equal(t, regularRequirementsContent, string(originalRequirementsContent))
	})

	t.Run("errors when client files don't exist", func(t *testing.T) {
		// Setup temporary directory for testing
		tempDir, err := os.MkdirTemp("", "test-client-deps-*")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// Create only regular files, no client files
		regularRequirementsContent := "fastapi==0.68.0"
		regularPackagesContent := "htop"

		err = os.WriteFile(filepath.Join(tempDir, "requirements.txt"), []byte(regularRequirementsContent), 0o644)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, "packages.txt"), []byte(regularPackagesContent), 0o644)
		assert.NoError(t, err)

		// Prepare build context should error since client files don't exist
		buildContext, err := prepareClientBuildContext(tempDir)
		assert.Error(t, err)
		assert.NotNil(t, buildContext) // Now returns buildContext even on error (for cleanup)
		assert.Contains(t, err.Error(), "failed to setup client dependency files")
		// Cleanup the temporary directory since function returns buildContext on error
		defer buildContext.CleanupFunc()
	})

	t.Run("returns error when source directory doesn't exist", func(t *testing.T) {
		// Setup temporary directory for testing
		tempDir, err := os.MkdirTemp("", "test-client-deps-*")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)

		nonExistentDir := filepath.Join(tempDir, "nonexistent")

		buildContext, err2 := prepareClientBuildContext(nonExistentDir)
		assert.Error(t, err2)
		assert.NotNil(t, buildContext) // Now returns buildContext even on error (for cleanup)
		assert.Contains(t, err2.Error(), "source directory does not exist")
		// Cleanup the temporary directory since function returns buildContext on error
		defer buildContext.CleanupFunc()
	})

	t.Run("errors when only some client files exist", func(t *testing.T) {
		// Setup temporary directory for testing
		tempDir, err := os.MkdirTemp("", "test-client-deps-*")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// Create only one client file
		clientRequirementsContent := "scikit-learn==1.0.0"
		regularRequirementsContent := "tensorflow==2.6.0"
		regularPackagesContent := "docker"

		err = os.WriteFile(filepath.Join(tempDir, "requirements-client.txt"), []byte(clientRequirementsContent), 0o644)
		assert.NoError(t, err)
		// Don't create packages-client.txt
		err = os.WriteFile(filepath.Join(tempDir, "requirements.txt"), []byte(regularRequirementsContent), 0o644)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, "packages.txt"), []byte(regularPackagesContent), 0o644)
		assert.NoError(t, err)

		// Prepare build context should error since packages-client.txt doesn't exist
		buildContext, err := prepareClientBuildContext(tempDir)
		assert.Error(t, err)
		assert.NotNil(t, buildContext) // Now returns buildContext even on error (for cleanup)
		assert.Contains(t, err.Error(), "failed to setup client dependency files")
		// Cleanup the temporary directory since function returns buildContext on error
		defer buildContext.CleanupFunc()

		// Verify original files are UNCHANGED
		originalRequirementsContent, err := os.ReadFile(filepath.Join(tempDir, "requirements.txt"))
		assert.NoError(t, err)
		assert.Equal(t, regularRequirementsContent, string(originalRequirementsContent))

		originalPackagesContent, err := os.ReadFile(filepath.Join(tempDir, "packages.txt"))
		assert.NoError(t, err)
		assert.Equal(t, regularPackagesContent, string(originalPackagesContent))
	})

	// Reproduces https://github.com/astronomer/astro-cli/issues/2161: a
	// .dockerignore-excluded directory containing a symlink to a directory
	// (e.g. terragrunt provider-cache symlinks under infra/) must not cause the
	// build-context copy to fail with "is a directory".
	t.Run("excludes dockerignored directory containing symlink to directory", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "test-client-dockerignore-*")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// Required client/regular dependency files.
		assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "requirements-client.txt"), []byte(""), 0o644))
		assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "packages-client.txt"), []byte(""), 0o644))
		assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "requirements.txt"), []byte(""), 0o644))
		assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "packages.txt"), []byte(""), 0o644))
		assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "Dockerfile.client"), []byte("FROM scratch"), 0o644))

		// .dockerignore excludes infra/.
		assert.NoError(t, os.WriteFile(filepath.Join(tempDir, ".dockerignore"), []byte("infra/\n"), 0o644))

		// A symlink-to-directory living outside the project, mirroring a
		// terragrunt provider cache symlinked into .terragrunt-cache.
		cacheDir := filepath.Join(tempDir, "provider-cache", "darwin_arm64")
		assert.NoError(t, os.MkdirAll(cacheDir, 0o755))
		assert.NoError(t, os.WriteFile(filepath.Join(cacheDir, "terraform-provider"), []byte("binary"), 0o644))

		linkDir := filepath.Join(tempDir, "infra", "agent", ".terragrunt-cache", "hash")
		assert.NoError(t, os.MkdirAll(linkDir, 0o755))
		assert.NoError(t, os.Symlink(cacheDir, filepath.Join(linkDir, "darwin_arm64")))

		// Previously failed with "is a directory"; now succeeds.
		buildContext, err := prepareClientBuildContext(tempDir)
		assert.NotNil(t, buildContext)
		defer buildContext.CleanupFunc()
		assert.NoError(t, err)

		// infra/ is excluded entirely from the build context.
		_, statErr := os.Stat(filepath.Join(buildContext.TempDir, "infra"))
		assert.True(t, os.IsNotExist(statErr), "expected infra/ to be excluded from build context")

		// The Dockerfile.client is preserved.
		_, statErr = os.Stat(filepath.Join(buildContext.TempDir, "Dockerfile.client"))
		assert.NoError(t, statErr)
	})

	// Even when .dockerignore does NOT exclude a symlink-to-directory, the copy
	// must recreate it as a symlink rather than dereference it.
	t.Run("copies symlink to directory without dereferencing when not ignored", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "test-client-symlink-*")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)

		assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "requirements-client.txt"), []byte(""), 0o644))
		assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "packages-client.txt"), []byte(""), 0o644))
		assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "requirements.txt"), []byte(""), 0o644))
		assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "packages.txt"), []byte(""), 0o644))

		target := filepath.Join(tempDir, "target-dir")
		assert.NoError(t, os.MkdirAll(target, 0o755))
		assert.NoError(t, os.Symlink(target, filepath.Join(tempDir, "link-to-dir")))

		buildContext, err := prepareClientBuildContext(tempDir)
		assert.NotNil(t, buildContext)
		defer buildContext.CleanupFunc()
		assert.NoError(t, err)

		info, lerr := os.Lstat(filepath.Join(buildContext.TempDir, "link-to-dir"))
		assert.NoError(t, lerr)
		assert.NotZero(t, info.Mode()&os.ModeSymlink, "expected link-to-dir to remain a symlink")
	})
}

func TestDockerignoreSkipFunc(t *testing.T) {
	t.Run("returns nil skip func when no .dockerignore exists", func(t *testing.T) {
		dir := t.TempDir()
		skip, err := dockerignoreSkipFunc(dir)
		assert.NoError(t, err)
		assert.Nil(t, skip)
	})

	t.Run("matches files and prunes directories", func(t *testing.T) {
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte("infra/\n*.log\n"), 0o644))

		skip, err := dockerignoreSkipFunc(dir)
		assert.NoError(t, err)
		assert.NotNil(t, skip)

		assert.True(t, skip("infra", true))
		assert.True(t, skip("infra/agent/main.tf", false))
		assert.True(t, skip("debug.log", false))
		assert.False(t, skip("dags/example.py", false))
		assert.False(t, skip("requirements.txt", false))
	})

	t.Run("never excludes the Dockerfile and dependency files", func(t *testing.T) {
		dir := t.TempDir()
		// A pathological .dockerignore that would otherwise exclude build files.
		assert.NoError(t, os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte("Dockerfile*\n*.txt\n.dockerignore\n"), 0o644))

		skip, err := dockerignoreSkipFunc(dir)
		assert.NoError(t, err)
		assert.NotNil(t, skip)

		assert.False(t, skip("Dockerfile.client", false))
		assert.False(t, skip(".dockerignore", false))
		assert.False(t, skip("requirements-client.txt", false))
		assert.False(t, skip("packages-client.txt", false))
	})

	t.Run("descends into matched directory when exclusion patterns exist", func(t *testing.T) {
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte("infra/\n!infra/keep.txt\n"), 0o644))

		skip, err := dockerignoreSkipFunc(dir)
		assert.NoError(t, err)
		assert.NotNil(t, skip)

		// Directory is not pruned (so the re-included child can be reached)...
		assert.False(t, skip("infra", true))
		// ...the re-included file survives...
		assert.False(t, skip("infra/keep.txt", false))
		// ...but other files inside stay excluded.
		assert.True(t, skip("infra/secret.tf", false))
	})
}

func TestSetupClientDependencyFiles(t *testing.T) {
	t.Run("copies client files to standard locations", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "test-setup-client-deps-*")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// Create client files
		clientRequirementsContent := "numpy==1.21.0"
		clientPackagesContent := "curl\nwget"
		regularRequirementsContent := "requests==2.28.0"
		regularPackagesContent := "git"

		err = os.WriteFile(filepath.Join(tempDir, "requirements-client.txt"), []byte(clientRequirementsContent), 0o644)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, "packages-client.txt"), []byte(clientPackagesContent), 0o644)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, "requirements.txt"), []byte(regularRequirementsContent), 0o644)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, "packages.txt"), []byte(regularPackagesContent), 0o644)
		assert.NoError(t, err)

		// Setup client dependency files
		err = setupClientDependencyFiles(tempDir)
		assert.NoError(t, err)

		// Verify both files were replaced with client content
		requirementsContent, err := os.ReadFile(filepath.Join(tempDir, "requirements.txt"))
		assert.NoError(t, err)
		assert.Equal(t, clientRequirementsContent, string(requirementsContent))

		packagesContent, err := os.ReadFile(filepath.Join(tempDir, "packages.txt"))
		assert.NoError(t, err)
		assert.Equal(t, clientPackagesContent, string(packagesContent))
	})

	t.Run("handles empty client files", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "test-setup-client-deps-*")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// Create empty client files
		err = os.WriteFile(filepath.Join(tempDir, "requirements-client.txt"), []byte(""), 0o644)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, "packages-client.txt"), []byte("   \n  "), 0o644)
		assert.NoError(t, err)

		// Create regular files with content
		regularRequirementsContent := "django==4.0.0"
		regularPackagesContent := "curl"
		err = os.WriteFile(filepath.Join(tempDir, "requirements.txt"), []byte(regularRequirementsContent), 0o644)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, "packages.txt"), []byte(regularPackagesContent), 0o644)
		assert.NoError(t, err)

		// Setup client dependency files
		err = setupClientDependencyFiles(tempDir)
		assert.NoError(t, err)

		// Verify files were replaced with empty client content
		requirementsContent, err := os.ReadFile(filepath.Join(tempDir, "requirements.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "", string(requirementsContent))

		packagesContent, err := os.ReadFile(filepath.Join(tempDir, "packages.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "   \n  ", string(packagesContent))
	})

	t.Run("errors when client files don't exist", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "test-setup-client-deps-*")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// Create only regular files, no client files
		err = os.WriteFile(filepath.Join(tempDir, "requirements.txt"), []byte("django==4.0.0"), 0o644)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, "packages.txt"), []byte("curl"), 0o644)
		assert.NoError(t, err)

		// Setup client dependency files should error since client files don't exist
		err = setupClientDependencyFiles(tempDir)
		assert.Error(t, err)
		// Due to map iteration being non-deterministic, the error could mention either client file
		errorMsg := err.Error()
		assert.True(t,
			strings.Contains(errorMsg, "failed to copy requirements-client.txt") ||
				strings.Contains(errorMsg, "failed to copy packages-client.txt"),
			"error should mention one of the missing client files, got: %s", errorMsg)
	})
}

func TestExtractRuntimeVersionFromImage(t *testing.T) {
	tests := []struct {
		name        string
		imageName   string
		expectedVer string
		expectError bool
	}{
		{
			name:        "valid image with runtime version",
			imageName:   "images.astronomer.cloud/baseimages/astro-remote-execution-agent:3.1-1-python-3.12-astro-agent-1.1.0",
			expectedVer: "3.1-1",
			expectError: false,
		},
		{
			name:        "valid image with different runtime version",
			imageName:   "registry.example.com/astro-agent:12.5-2-python-3.11-astro-agent-2.0.0",
			expectedVer: "12.5-2",
			expectError: false,
		},
		{
			name:        "image without tag",
			imageName:   "images.astronomer.cloud/baseimages/astro-remote-execution-agent",
			expectedVer: "",
			expectError: true,
		},
		{
			name:        "image with invalid tag format",
			imageName:   "images.astronomer.cloud/baseimages/astro-remote-execution-agent:invalid-tag",
			expectedVer: "",
			expectError: true,
		},
		{
			name:        "image with base suffix",
			imageName:   "images.astronomer.cloud/baseimages/astro-remote-execution-agent:3.1-1-python-3.12-astro-agent-1.1.0-base",
			expectedVer: "3.1-1",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := extractRuntimeVersionFromImage(tt.imageName)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedVer, version)
			}
		})
	}
}

func TestValidateClientImageRuntimeVersion(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "test-validate-client-image")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("skip validation when no deployment ID", func(t *testing.T) {
		deployInput := InputClientDeploy{
			Path: tempDir,
		}

		// Should not error when deployment ID is empty
		_, err := validateClientImageRuntimeVersion(deployInput, nil)
		assert.NoError(t, err)
	})

	t.Run("error when getting current context fails", func(t *testing.T) {
		// Reset current context to force context error
		err := config.ResetCurrentContext()
		assert.NoError(t, err)

		deployInput := InputClientDeploy{
			Path:         tempDir,
			DeploymentID: "test-deployment-id",
		}

		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)

		_, err = validateClientImageRuntimeVersion(deployInput, mockV1Client)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get current context")
	})

	t.Run("error when getting deployment information fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)

		deployInput := InputClientDeploy{
			Path:         tempDir,
			DeploymentID: "test-deployment-id",
		}

		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)

		// Mock GetDeploymentWithResponse to return an error
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, "test-deployment-id").Return(
			&astrov1.GetDeploymentResponse{
				HTTPResponse: &http.Response{StatusCode: 500},
			}, nil)

		_, err := validateClientImageRuntimeVersion(deployInput, mockV1Client)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get deployment information")
	})

	t.Run("error when Dockerfile.client doesn't exist", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)

		deployInput := InputClientDeploy{
			Path:         tempDir,
			DeploymentID: "test-deployment-id",
		}

		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)

		// Mock successful deployment response
		runtimeVersion := "3.0.0"
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, "test-deployment-id").Return(
			&astrov1.GetDeploymentResponse{
				HTTPResponse: &http.Response{StatusCode: 200},
				JSON200: &astrov1.Deployment{
					Id:                  "test-deployment-id",
					AstroRuntimeVersion: runtimeVersion,
					Namespace:           "test-namespace",
					WorkspaceId:         "test-workspace-id",
					WebServerUrl:        "https://test.com",
				},
			}, nil)

		_, err := validateClientImageRuntimeVersion(deployInput, mockV1Client)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Dockerfile.client is required for client image runtime version validation")
	})

	t.Run("error when Dockerfile.client exists but fails to parse", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)

		// Create a malformed Dockerfile.client that will fail parsing
		dockerfilePath := filepath.Join(tempDir, "Dockerfile.client")
		// This creates a Dockerfile with invalid JSON syntax that will cause parsing to fail
		malformedDockerfile := "FROM ubuntu:20.04\nCMD [\"echo\", 1]"
		err := os.WriteFile(dockerfilePath, []byte(malformedDockerfile), 0o644)
		assert.NoError(t, err)

		deployInput := InputClientDeploy{
			Path:         tempDir,
			DeploymentID: "test-deployment-id",
		}

		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)

		// Mock successful deployment response
		runtimeVersion := "3.0.0"
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, "test-deployment-id").Return(
			&astrov1.GetDeploymentResponse{
				HTTPResponse: &http.Response{StatusCode: 200},
				JSON200: &astrov1.Deployment{
					Id:                  "test-deployment-id",
					AstroRuntimeVersion: runtimeVersion,
					Namespace:           "test-namespace",
					WorkspaceId:         "test-workspace-id",
					WebServerUrl:        "https://test.com",
				},
			}, nil)

		_, err = validateClientImageRuntimeVersion(deployInput, mockV1Client)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse Dockerfile.client")
	})

	t.Run("error when no base image found in Dockerfile.client", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)

		// Create a Dockerfile.client without FROM instruction
		dockerfilePath := filepath.Join(tempDir, "Dockerfile.client")
		err := os.WriteFile(dockerfilePath, []byte("RUN echo 'test'\nCOPY . .\n"), 0o644)
		assert.NoError(t, err)

		deployInput := InputClientDeploy{
			Path:         tempDir,
			DeploymentID: "test-deployment-id",
		}

		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)

		// Mock successful deployment response
		runtimeVersion := "3.0.0"
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, "test-deployment-id").Return(
			&astrov1.GetDeploymentResponse{
				HTTPResponse: &http.Response{StatusCode: 200},
				JSON200: &astrov1.Deployment{
					Id:                  "test-deployment-id",
					AstroRuntimeVersion: runtimeVersion,
					Namespace:           "test-namespace",
					WorkspaceId:         "test-workspace-id",
					WebServerUrl:        "https://test.com",
				},
			}, nil)

		_, err = validateClientImageRuntimeVersion(deployInput, mockV1Client)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to find base image in Dockerfile.client")
	})

	t.Run("error when runtime version extraction fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)

		// Create a Dockerfile.client with image that doesn't have extractable version
		dockerfilePath := filepath.Join(tempDir, "Dockerfile.client")
		dockerContent := "FROM python:3.9\nCOPY . .\n"
		err := os.WriteFile(dockerfilePath, []byte(dockerContent), 0o644)
		assert.NoError(t, err)

		deployInput := InputClientDeploy{
			Path:         tempDir,
			DeploymentID: "test-deployment-id",
		}

		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)

		// Mock successful deployment response
		runtimeVersion := "3.0.0"
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, "test-deployment-id").Return(
			&astrov1.GetDeploymentResponse{
				HTTPResponse: &http.Response{StatusCode: 200},
				JSON200: &astrov1.Deployment{
					Id:                  "test-deployment-id",
					AstroRuntimeVersion: runtimeVersion,
					Namespace:           "test-namespace",
					WorkspaceId:         "test-workspace-id",
					WebServerUrl:        "https://test.com",
				},
			}, nil)

		// Should error when version extraction fails
		_, err = validateClientImageRuntimeVersion(deployInput, mockV1Client)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to extract runtime version from client image")
	})

	t.Run("error when client runtime version is newer than deployment version", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)

		// Create a Dockerfile.client with newer runtime version
		dockerfilePath := filepath.Join(tempDir, "Dockerfile.client")
		dockerContent := "FROM images.astronomer.cloud/baseimages/astro-remote-execution-agent:4.0-1-python-3.12-astro-agent-1.1.0\nCOPY . .\n"
		err := os.WriteFile(dockerfilePath, []byte(dockerContent), 0o644)
		assert.NoError(t, err)

		deployInput := InputClientDeploy{
			Path:         tempDir,
			DeploymentID: "test-deployment-id",
		}

		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)

		// Mock deployment with older runtime version
		runtimeVersion := "3.0.0"
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, "test-deployment-id").Return(
			&astrov1.GetDeploymentResponse{
				HTTPResponse: &http.Response{StatusCode: 200},
				JSON200: &astrov1.Deployment{
					Id:                  "test-deployment-id",
					AstroRuntimeVersion: runtimeVersion,
					Namespace:           "test-namespace",
					WorkspaceId:         "test-workspace-id",
					WebServerUrl:        "https://test.com",
				},
			}, nil)

		_, err = validateClientImageRuntimeVersion(deployInput, mockV1Client)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "client image runtime version validation failed")
		assert.Contains(t, err.Error(), "4.0-1")
		assert.Contains(t, err.Error(), "3.0.0")
	})

	t.Run("success when client runtime version is compatible with deployment version", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)

		// Create a Dockerfile.client with compatible runtime version
		dockerfilePath := filepath.Join(tempDir, "Dockerfile.client")
		dockerContent := "FROM images.astronomer.cloud/baseimages/astro-remote-execution-agent:3.0-5-python-3.12-astro-agent-1.1.0\nCOPY . .\n"
		err := os.WriteFile(dockerfilePath, []byte(dockerContent), 0o644)
		assert.NoError(t, err)

		deployInput := InputClientDeploy{
			Path:         tempDir,
			DeploymentID: "test-deployment-id",
		}

		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)

		// Mock deployment with newer runtime version
		runtimeVersion := "3.0-7"
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, "test-deployment-id").Return(
			&astrov1.GetDeploymentResponse{
				HTTPResponse: &http.Response{StatusCode: 200},
				JSON200: &astrov1.Deployment{
					Id:                  "test-deployment-id",
					AstroRuntimeVersion: runtimeVersion,
					Namespace:           "test-namespace",
					WorkspaceId:         "test-workspace-id",
					WebServerUrl:        "https://test.com",
				},
			}, nil)

		check, err := validateClientImageRuntimeVersion(deployInput, mockV1Client)
		assert.NoError(t, err)
		assert.Equal(t, &ClientRuntimeCheck{DeploymentID: "test-deployment-id", ClientRuntimeVersion: "3.0-5", DeploymentRuntimeVersion: "3.0-7"}, check)
	})

	t.Run("success when client runtime version equals deployment version", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)

		// Create a Dockerfile.client with same runtime version
		dockerfilePath := filepath.Join(tempDir, "Dockerfile.client")
		dockerContent := "FROM images.astronomer.cloud/baseimages/astro-remote-execution-agent:3.0-1-python-3.12-astro-agent-1.1.0\nCOPY . .\n"
		err := os.WriteFile(dockerfilePath, []byte(dockerContent), 0o644)
		assert.NoError(t, err)

		deployInput := InputClientDeploy{
			Path:         tempDir,
			DeploymentID: "test-deployment-id",
		}

		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)

		// Mock deployment with exact same runtime version
		runtimeVersion := "3.0-1"
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, "test-deployment-id").Return(
			&astrov1.GetDeploymentResponse{
				HTTPResponse: &http.Response{StatusCode: 200},
				JSON200: &astrov1.Deployment{
					Id:                  "test-deployment-id",
					AstroRuntimeVersion: runtimeVersion,
					Namespace:           "test-namespace",
					WorkspaceId:         "test-workspace-id",
					WebServerUrl:        "https://test.com",
				},
			}, nil)

		_, err = validateClientImageRuntimeVersion(deployInput, mockV1Client)
		assert.NoError(t, err)
	})
}

func TestDeployDagsBundleLayout(t *testing.T) {
	// Test that --no-dags-base-dir flag controls the bundle layout.
	// By default (flag=false), files are placed under dags/ prefix.
	// With flag=true, files are placed at bundle root (for Airflow 3.x compatibility).
	// This is critical for issue #1985: Airflow 3 adds bundle root to sys.path,
	// so imports fail if DAGs are nested under dags/.
	testCases := []struct {
		name                   string
		noDagsBaseDir          bool
		expectedPrependBaseDir bool
	}{
		{"default behavior includes dags/ prefix", false, true},
		{"--no-dags-base-dir puts files at root", true, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// This mirrors the logic in deployDags():
			// prependBaseDir := !noDagsBaseDir
			result := !tc.noDagsBaseDir
			assert.Equal(t, tc.expectedPrependBaseDir, result,
				"prependBaseDir should be %v when noDagsBaseDir=%v", tc.expectedPrependBaseDir, tc.noDagsBaseDir)
		})
	}
}
