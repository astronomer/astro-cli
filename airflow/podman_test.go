package airflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/astronomer/astro-cli/airflow/mocks"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/messages"
	testUtils "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/cri-o/ocicni/pkg/ocicni"

	"github.com/containers/podman/v3/pkg/bindings/containers"
	"github.com/containers/podman/v3/pkg/domain/entities"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	errPodman            = errors.New("some podman error")
	errContainerNotFound = errors.New(messages.ErrContainerNotFound)
)

func TestPodmanGetConnSuccess(t *testing.T) {
	bindMock := new(mocks.PodmanBind)
	bindMock.On("NewConnection", mock.Anything, mock.Anything).Return(context.TODO(), nil)
	expectedConn, err := getConn(context.TODO(), bindMock)
	assert.NoError(t, err)
	assert.Equal(t, context.TODO(), expectedConn)
	bindMock.AssertExpectations(t)
}

func TestPodmanPSSuccess(t *testing.T) {
	mockResp := []entities.ListContainer{
		{Names: []string{"webserver"}, State: "running", Ports: []ocicni.PortMapping{{HostPort: 8080, ContainerPort: 8080}}},
		{Names: []string{"scheduler"}, State: "running", Ports: []ocicni.PortMapping{{HostPort: 8080, ContainerPort: 8080}}},
		{Names: []string{"postgres"}, State: "running", Ports: []ocicni.PortMapping{{HostPort: 5432, ContainerPort: 5432}}},
	}
	bindMock := new(mocks.PodmanBind)
	podmanMock := &Podman{projectDir: "test", projectName: "test", envFile: ".env", podmanBind: bindMock, conn: context.TODO()}
	bindMock.On("List", podmanMock.conn, mock.Anything).Return(mockResp, nil)
	err := podmanMock.PS()
	assert.NoError(t, err)
	bindMock.AssertExpectations(t)
}

func TestPodmanPSFailure(t *testing.T) {
	bindMock := new(mocks.PodmanBind)
	podmanMock := &Podman{projectDir: "test", projectName: "test", envFile: ".env", podmanBind: bindMock, conn: context.TODO()}
	bindMock.On("List", podmanMock.conn, mock.Anything).Return([]entities.ListContainer{}, errPodman)
	err := podmanMock.PS()
	assert.Equal(t, errPodman, err)
	bindMock.AssertExpectations(t)
}

func TestPodmanGetContainerIDFailure(t *testing.T) {
	bindMock := new(mocks.PodmanBind)
	podmanMock := &Podman{projectDir: "test", projectName: "test", envFile: ".env", podmanBind: bindMock, conn: context.TODO()}
	bindMock.On("List", podmanMock.conn, mock.Anything).Return([]entities.ListContainer{}, nil)
	resp, err := podmanMock.GetContainerID("test-container")
	assert.Equal(t, "", resp)
	assert.Equal(t, errContainerNotFound.Error(), err.Error())
	bindMock.AssertExpectations(t)

	bindMock = new(mocks.PodmanBind)
	bindMock.On("List", podmanMock.conn, mock.Anything).Return([]entities.ListContainer{{Names: []string{"scheduler"}, ID: "test-id"}}, nil)
	podmanMock = &Podman{projectDir: "test", projectName: "test", envFile: ".env", podmanBind: bindMock, conn: context.TODO()}
	resp, err = podmanMock.GetContainerID("test-container")
	assert.Equal(t, "", resp)
	assert.Equal(t, errContainerNotFound.Error(), err.Error())
	bindMock.AssertExpectations(t)

	bindMock = new(mocks.PodmanBind)
	bindMock.On("List", podmanMock.conn, mock.Anything).Return([]entities.ListContainer{{Names: []string{"scheduler"}, ID: "test-id"}}, errPodman)
	podmanMock = &Podman{projectDir: "test", projectName: "test", envFile: ".env", podmanBind: bindMock, conn: context.TODO()}
	resp, err = podmanMock.GetContainerID("test-container")
	assert.Equal(t, "", resp)
	assert.Equal(t, errPodman.Error(), err.Error())
	bindMock.AssertExpectations(t)
}

func TestPodmanStartSuccess(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig()
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)
	projectDir, _ := os.Getwd()

	mockResponse := []byte("Connection successful.")

	bindMock := new(mocks.PodmanBind)
	bindMock.On("NewConnection", mock.Anything, mock.Anything).Return(context.TODO(), nil)
	podmanMock := &Podman{projectDir: "test", projectName: "test", envFile: ".env", podmanBind: bindMock, conn: context.TODO()}
	bindMock.On("List", podmanMock.conn, mock.Anything).Return([]entities.ListContainer{}, nil).Once()
	bindMock.On("Build", podmanMock.conn, []string{filepath.Join(projectDir, "Dockerfile")}, mock.AnythingOfType("entities.BuildOptions")).Return(&entities.BuildReport{}, nil)
	bindMock.On("Exists", podmanMock.conn, mock.Anything, mock.Anything).Return(false, nil)
	bindMock.On("Kube", podmanMock.conn, mock.Anything, mock.Anything).Return(&entities.PlayKubeReport{Pods: []entities.PlayKubePod{{ID: "test"}}}, nil)
	bindMock.On("List", podmanMock.conn, mock.Anything).Return([]entities.ListContainer{{Names: []string{"test-webserver"}, ID: "test-id"}}, nil)
	bindMock.On("ExecCreate", podmanMock.conn, mock.Anything, mock.Anything).Return("test-exec-id", nil)

	respCounter := 0
	mockCall := bindMock.On("ExecStartAndAttach", podmanMock.conn, mock.Anything, mock.Anything)
	mockCall.RunFn = func(args mock.Arguments) {
		if respCounter >= 1 {
			streams := args.Get(2).(*containers.ExecStartAndAttachOptions)
			streams.GetOutputStream().Write(mockResponse)
		}
		respCounter++
		mockCall.ReturnArguments = mock.Arguments{nil}
	}

	err := podmanMock.Start("test-dockerfile")
	assert.NoError(t, err)

	// Case when pod is already present but in stop state
	err = podmanMock.Start("test-dockerfile")
	assert.NoError(t, err)
}

func TestPodmanStartFailure(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig()
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)
	projectDir, _ := os.Getwd()

	bindMock := new(mocks.PodmanBind)
	bindMock.On("NewConnection", mock.Anything, mock.Anything).Return(context.TODO(), nil)
	podmanMock := &Podman{projectDir: "test", projectName: "test", envFile: ".env", podmanBind: bindMock, conn: context.TODO()}
	bindMock.On("List", podmanMock.conn, mock.Anything).Return([]entities.ListContainer{{Names: []string{"test-webserver"}, ID: "test-id", State: "running"}}, nil).Once()

	err := podmanMock.Start("test-dockerfile")
	assert.EqualError(t, err, "cannot start, project already running")

	bindMock.On("List", podmanMock.conn, mock.Anything).Return([]entities.ListContainer{}, nil).Once()
	bindMock.On("Build", podmanMock.conn, []string{filepath.Join(projectDir, "Dockerfile")}, mock.AnythingOfType("entities.BuildOptions")).Return(nil, errPodman).Once()
	err = podmanMock.Start("test-dockerfile")
	assert.EqualError(t, err, errPodman.Error())

	bindMock.On("List", podmanMock.conn, mock.Anything).Return([]entities.ListContainer{}, nil).Once()
	bindMock.On("Build", podmanMock.conn, []string{filepath.Join(projectDir, "Dockerfile")}, mock.AnythingOfType("entities.BuildOptions")).Return(&entities.BuildReport{}, nil)
	bindMock.On("Exists", podmanMock.conn, mock.Anything, mock.Anything).Return(false, nil).Once()
	bindMock.On("Kube", podmanMock.conn, mock.Anything, mock.Anything).Return(nil, errPodman).Once()
	err = podmanMock.Start("test-dockerfile")
	assert.Contains(t, err.Error(), errPodman.Error())

	bindMock.On("List", podmanMock.conn, mock.Anything).Return([]entities.ListContainer{}, nil).Once()
	bindMock.On("Exists", podmanMock.conn, mock.Anything, mock.Anything).Return(true, nil).Once()
	bindMock.On("Start", podmanMock.conn, mock.Anything, mock.Anything).Return(nil, errPodman).Once()
	err = podmanMock.Start("test-dockerfile")
	assert.Contains(t, err.Error(), errPodman.Error())
}

func TestPodmanKillSuccess(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig()
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	mockResp := &entities.PlayKubeReport{
		Pods:    []entities.PlayKubePod{{ID: "test-1", Containers: []string{"websever-id", "scheduler-id", "postgres-id"}}},
		Volumes: []entities.PlayKubeVolume{{Name: "test-volume"}},
	}

	bindMock := new(mocks.PodmanBind)
	bindMock.On("NewConnection", mock.Anything, mock.Anything).Return(context.TODO(), nil)
	podmanMock := &Podman{projectDir: "test", projectName: "test", envFile: ".env", podmanBind: bindMock, conn: context.TODO()}
	bindMock.On("KubeDown", podmanMock.conn, mock.Anything).Return(mockResp, nil).Once()

	err := podmanMock.Kill()
	assert.NoError(t, err)
}

func TestPodmanKillFailure(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig()
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	bindMock := new(mocks.PodmanBind)
	bindMock.On("NewConnection", mock.Anything, mock.Anything).Return(context.TODO(), nil)
	podmanMock := &Podman{projectDir: "test", projectName: "test", envFile: ".env", podmanBind: bindMock, conn: context.TODO()}
	bindMock.On("KubeDown", podmanMock.conn, mock.Anything).Return(&entities.PlayKubeReport{}, errPodman).Once()

	err := podmanMock.Kill()
	assert.Contains(t, err.Error(), errPodman.Error())
}

func TestPodmanLogsSuccess(t *testing.T) {
	mockResp := []entities.ListContainer{
		{ID: "test-1", Names: []string{"airflow-webserver"}, State: "running", Ports: []ocicni.PortMapping{{HostPort: 8080, ContainerPort: 8080}}},
		{ID: "test-2", Names: []string{"airflow-scheduler"}, State: "running", Ports: []ocicni.PortMapping{{HostPort: 8080, ContainerPort: 8080}}},
		{ID: "test-3", Names: []string{"airflow-postgres"}, State: "running", Ports: []ocicni.PortMapping{{HostPort: 5432, ContainerPort: 5432}}},
	}
	bindMock := new(mocks.PodmanBind)
	podmanMock := &Podman{projectDir: "test", projectName: "test", envFile: ".env", podmanBind: bindMock, conn: context.TODO()}
	bindMock.On("List", podmanMock.conn, mock.Anything).Return(mockResp, nil)

	mockCall := bindMock.On("Logs", podmanMock.conn, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockCall.RunFn = func(args mock.Arguments) {
		stdoutStream := args.Get(3).(chan string)
		stderrStream := args.Get(4).(chan string)
		stdoutStream <- "test log message to stdout"
		stderrStream <- "test error log message to stderr"
		mockCall.ReturnArguments = mock.Arguments{nil}
	}

	err := podmanMock.Logs(true, []string{"webserver", "no-logs-container"}...)
	assert.NoError(t, err)
}

func TestPodmanLogsFailure(t *testing.T) {
	mockResp := []entities.ListContainer{}
	bindMock := new(mocks.PodmanBind)
	podmanMock := &Podman{projectDir: "test", projectName: "test", envFile: ".env", podmanBind: bindMock, conn: context.TODO()}
	bindMock.On("List", podmanMock.conn, mock.Anything).Return(mockResp, nil).Once()

	err := podmanMock.Logs(true, []string{"webserver"}...)
	assert.Contains(t, err.Error(), "cannot view logs, project not running")

	mockResp = []entities.ListContainer{
		{ID: "test-1", Names: []string{"airflow-webserver"}, State: "running", Ports: []ocicni.PortMapping{{HostPort: 8080, ContainerPort: 8080}}},
		{ID: "test-2", Names: []string{"airflow-scheduler"}, State: "running", Ports: []ocicni.PortMapping{{HostPort: 8080, ContainerPort: 8080}}},
		{ID: "test-3", Names: []string{"airflow-postgres"}, State: "running", Ports: []ocicni.PortMapping{{HostPort: 5432, ContainerPort: 5432}}},
	}
	bindMock.On("List", podmanMock.conn, mock.Anything).Return(mockResp, nil)
	bindMock.On("Logs", podmanMock.conn, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errPodman)

	err = podmanMock.Logs(true, []string{"webserver"}...)
	assert.Contains(t, err.Error(), errPodman.Error())
}

func TestPodmanStopSuccess(t *testing.T) {
	bindMock := new(mocks.PodmanBind)
	podmanMock := &Podman{projectDir: "test", projectName: "test", envFile: ".env", podmanBind: bindMock, conn: context.TODO()}
	bindMock.On("Stop", podmanMock.conn, mock.Anything, mock.Anything).Return(&entities.PodStopReport{}, nil)

	err := podmanMock.Stop()
	assert.NoError(t, err)
}

func TestPodmanStopFailure(t *testing.T) {
	bindMock := new(mocks.PodmanBind)
	podmanMock := &Podman{projectDir: "test", projectName: "test", envFile: ".env", podmanBind: bindMock, conn: context.TODO()}
	bindMock.On("Stop", podmanMock.conn, mock.Anything, mock.Anything).Return(&entities.PodStopReport{}, errPodman)

	err := podmanMock.Stop()
	assert.Contains(t, err.Error(), errPodman.Error())
}

func TestPodmanRunSuccess(t *testing.T) {
	bindMock := new(mocks.PodmanBind)
	podmanMock := &Podman{projectDir: "test", projectName: "test", envFile: ".env", podmanBind: bindMock, conn: context.TODO()}
	bindMock.On("List", podmanMock.conn, mock.Anything).Return([]entities.ListContainer{{Names: []string{"test-webserver"}, ID: "test-id", State: "running"}}, nil).Once()
	bindMock.On("ExecCreate", podmanMock.conn, mock.Anything, mock.Anything).Return("test-exec-id", nil)

	mockResponse := "Connection successful.\n"
	mockCall := bindMock.On("ExecStartAndAttach", podmanMock.conn, mock.Anything, mock.Anything)
	mockCall.RunFn = func(args mock.Arguments) {
		streams := args.Get(2).(*containers.ExecStartAndAttachOptions)
		streams.GetOutputStream().Write([]byte(mockResponse))
		mockCall.ReturnArguments = mock.Arguments{nil}
	}

	err := podmanMock.Run([]string{"db", "check"}, "user")
	assert.NoError(t, err)
}

func TestPodmanRunFailure(t *testing.T) {
	bindMock := new(mocks.PodmanBind)
	podmanMock := &Podman{projectDir: "test", projectName: "test", envFile: ".env", podmanBind: bindMock, conn: context.TODO()}
	bindMock.On("List", podmanMock.conn, mock.Anything).Return([]entities.ListContainer{{Names: []string{"test-webserver"}, ID: "test-id", State: "running"}}, nil).Once()
	bindMock.On("ExecCreate", podmanMock.conn, mock.Anything, mock.Anything).Return("test-exec-id", nil)
	bindMock.On("ExecStartAndAttach", podmanMock.conn, mock.Anything, mock.Anything).Return(errPodman)

	err := podmanMock.Run([]string{"db", "check"}, "user")
	assert.Contains(t, err.Error(), errPodman.Error())
}
