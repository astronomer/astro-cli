package sql

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/stdcopy"
)

const SQL_CLI_DOCKERFILE_PATH_ENV_VAR = "SQL_CLI_DOCKERFILE_PATH"

func getContext(filePath string) io.Reader {
	ctx, _ := archive.TarWithOptions(filePath, &archive.TarOptions{})
	return ctx
}

func CommonDockerUtil(cmd []string, flags map[string]string) error {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		err = fmt.Errorf("Docker client initialisation failed %w", err)
		return err
	}

	astroSQLCliVersion := GetPypiVersion(ASTRO_SQL_CLI_PROJECT_URL)
	opts := types.ImageBuildOptions{
		Dockerfile: "Dockerfile.sql_cli",
		Tags:       []string{"sql_cli"},
	}

	if astroSQLCliVersion != "" {
		opts.BuildArgs = map[string]*string{"VERSION": &astroSQLCliVersion}
	}

	dockerfilePath := os.Getenv(SQL_CLI_DOCKERFILE_PATH_ENV_VAR)
	if dockerfilePath == "" {
		err = fmt.Errorf("Environment variable %s not set", SQL_CLI_DOCKERFILE_PATH_ENV_VAR)
		return err
	}

	body, err := cli.ImageBuild(ctx, getContext(dockerfilePath), opts)
	if err != nil {
		err = fmt.Errorf("Image building failed %w ", err)
		return err
	}
	buf := new(strings.Builder)
	_, err = io.Copy(buf, body.Body)
	if err != nil {
		err = fmt.Errorf("Image build response read failed %w", err)
		return err
	}

	for key, value := range flags {
		cmd = append(cmd, []string{fmt.Sprintf("--%s", key), value}...)
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: "sql_cli",
		Cmd:   cmd,
		Tty:   false,
	}, nil, nil, nil, "")
	if err != nil {
		err = fmt.Errorf("Docker container creation failed %w", err)
		return err
	}

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		err = fmt.Errorf("Docker container start failed %w", err)
		return err
	}

	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			err = fmt.Errorf("Docker client run failed %w", err)
			return err
		}
	case <-statusCh:
	}

	cout, err := cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStdout: true})
	if err != nil {
		err = fmt.Errorf("Docker container logs fetching failed %w", err)
		return err
	}

	_, err = stdcopy.StdCopy(os.Stdout, os.Stderr, cout)

	if err != nil {
		err = fmt.Errorf("Docker logs forwarding failed %w", err)
		return err
	}

	return nil
}
