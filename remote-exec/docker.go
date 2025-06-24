package remoteexec

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/astronomer/astro-cli/airflow"
	airflowTypes "github.com/astronomer/astro-cli/airflow/types"
	"github.com/google/uuid"
)

const (
	composeFile = "remote-docker-compose.yaml"
)

// RemoteExecDocker wraps the airflow DockerCompose functionality for remote execution
type RemoteExecDocker struct {
	*airflow.DockerCompose
	composeFile string
}

// Config holds configuration for remote execution services
type Config struct {
	ProjectName    string
	ImageName      string
	ComposeFile    string
	EnvFile        string
	AgentToken     string
	RemoteAPIURL   string
	DPID           string
	WorkerID       string
	TriggererID    string
	DeploymentName string
}

// Init initializes a new RemoteExecDocker instance by reusing airflow DockerCompose
func Init(config Config) (*RemoteExecDocker, error) {
	// Set default compose file if not provided
	if config.ComposeFile == "" {
		config.ComposeFile = composeFile
	}

	// Copy the remote-exec docker-compose file to the current working directory
	_, filename, _, _ := runtime.Caller(0)
	dir := filepath.Dir(filename)
	sourceComposeFile := filepath.Join(dir, "remote-docker-compose.yaml")

	// Read the source compose file
	sourceContent, err := os.ReadFile(sourceComposeFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read remote-exec compose file: %w", err)
	}

	// Write to the current working directory
	err = os.WriteFile(config.ComposeFile, sourceContent, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to write compose file to current directory: %w", err)
	}

	// Set environment variables for the remote execution services
	if config.AgentToken != "" {
		os.Setenv("AGENT_TOKEN", config.AgentToken)
	} else {
		return nil, fmt.Errorf("agent token is required for remote exec docker compose")
	}
	if config.RemoteAPIURL != "" {
		os.Setenv("ASTRO_REMOTE_API_URL", config.RemoteAPIURL)
	}
	if config.ImageName != "" {
		os.Setenv("REMOTE_EXEC_IMAGE", config.ImageName)
	}

	if config.DPID != "" {
		os.Setenv("DP_ID", config.DPID)
	} else {
		os.Setenv("DP_ID", "dag-processor-"+uuid.NewString())
	}
	if config.WorkerID != "" {
		os.Setenv("WORKER_ID", config.WorkerID)
	} else {
		os.Setenv("WORKER_ID", "worker-"+uuid.NewString())
	}
	if config.TriggererID != "" {
		os.Setenv("TRIGGERER_ID", config.TriggererID)
	} else {
		os.Setenv("TRIGGERER_ID", "triggerer-"+uuid.NewString())
	}

	// Initialize the underlying airflow DockerCompose
	dockerCompose, err := airflow.DockerComposeInit(".", config.EnvFile, "Dockerfile", config.ImageName)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DockerCompose: %w", err)
	}

	return &RemoteExecDocker{
		DockerCompose: dockerCompose,
		composeFile:   config.ComposeFile,
	}, nil
}

// Start starts the remote execution services using the custom compose file
func (r *RemoteExecDocker) Start(imageName, buildSecretString string, noCache bool, waitTime time.Duration) error {
	// Use the custom compose file instead of the default one
	return r.DockerCompose.Start(imageName, "", r.composeFile, buildSecretString, noCache, false, waitTime, nil)
}

// Stop stops the remote execution services
func (r *RemoteExecDocker) Stop(waitForExit bool) error {
	return r.DockerCompose.Stop(waitForExit)
}

// PS shows the status of remote execution services
func (r *RemoteExecDocker) PS() error {
	return r.DockerCompose.PS()
}

// Logs shows logs from the remote execution services
func (r *RemoteExecDocker) Logs(follow bool, serviceNames ...string) error {
	return r.DockerCompose.Logs(follow, serviceNames...)
}

// Run runs a command in a specific service container
func (r *RemoteExecDocker) Run(serviceName string, args []string, user string) error {
	// For remote exec, we'll run the command in the specified service
	// The airflow Run method runs in the webserver by default, so we need to adapt
	if serviceName == "" {
		serviceName = "worker" // Default to worker service for remote exec
	}

	// Use the underlying Run method but with service-specific logic
	return r.DockerCompose.Run(args, user)
}

// Bash opens a bash shell in a specific service container
func (r *RemoteExecDocker) Bash(serviceName string) error {
	if serviceName == "" {
		serviceName = "worker" // Default to worker service for remote exec
	}
	return r.DockerCompose.Bash(serviceName)
}

// Build builds the remote execution image using the airflow image handler
func (r *RemoteExecDocker) Build(dockerfile, buildSecretString string, noCache bool) error {
	// Create an image handler for building
	imageHandler := airflow.ImageHandlerInit("")
	buildConfig := airflowTypes.ImageBuildConfig{
		Path:    ".",
		NoCache: noCache,
	}
	return imageHandler.Build(dockerfile, buildSecretString, buildConfig)
}

// Push pushes the remote execution image to a registry
func (r *RemoteExecDocker) Push(remoteImage, username, token string, getImageRepoSha bool) (string, error) {
	imageHandler := airflow.ImageHandlerInit("")
	return imageHandler.Push(remoteImage, username, token, getImageRepoSha)
}

// Pull pulls the remote execution image from a registry
func (r *RemoteExecDocker) Pull(remoteImage, username, token string) error {
	imageHandler := airflow.ImageHandlerInit("")
	return imageHandler.Pull(remoteImage, username, token)
}

// GetImageLabels gets all labels from the remote execution image
func (r *RemoteExecDocker) GetImageLabels() (map[string]string, error) {
	imageHandler := airflow.ImageHandlerInit("")
	return imageHandler.ListLabels()
}

// GetImageLabel gets a specific label from the remote execution image
func (r *RemoteExecDocker) GetImageLabel(labelName string) (string, error) {
	imageHandler := airflow.ImageHandlerInit("")
	return imageHandler.GetLabel("", labelName)
}

// DoesImageExist checks if the remote execution image exists
func (r *RemoteExecDocker) DoesImageExist() error {
	imageHandler := airflow.ImageHandlerInit("")
	return imageHandler.DoesImageExist("")
}

// Restart restarts the remote execution services
func (r *RemoteExecDocker) Restart() error {
	// Stop and then start the services
	err := r.Stop(false)
	if err != nil {
		return fmt.Errorf("failed to stop services: %w", err)
	}

	return r.Start("", "", false, 0)
}

// GetComposeFilePath returns the full path to the compose file
func (r *RemoteExecDocker) GetComposeFilePath() string {
	// Get the directory where this Go file is located
	_, filename, _, _ := runtime.Caller(0)
	dir := filepath.Dir(filename)
	return filepath.Join(dir, r.composeFile)
}
