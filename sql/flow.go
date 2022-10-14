package sql

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/stdcopy"
)

func getContext(filePath string) io.Reader {
	ctx, _ := archive.TarWithOptions(filePath, &archive.TarOptions{})
	return ctx
}

func CommonDockerUtil(cmd []string, flags map[string]string) error {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	opts := types.ImageBuildOptions{
		Dockerfile: "Dockerfile.sql_cli",
		Tags:       []string{"sql_cli6"},
		NoCache:    true,
	}

	_, currentfilePath, _, _ := runtime.Caller(0)
	dockerfilePath := path.Join(path.Dir(currentfilePath), "../Dockerfile.sql_cli")
	body, err := cli.ImageBuild(ctx, getContext(dockerfilePath), opts)
	buf := new(strings.Builder)
	_, err = io.Copy(buf, body.Body)
	// check errors
	fmt.Println(buf.String())
	fmt.Println(body, err)
	if err != nil {
		panic(err)
	}
	for key, value := range flags {
		cmd = append(cmd, []string{fmt.Sprintf("--%s", key), value}...)
	}
	fmt.Println(cmd)
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: "sql_cli6",
		Cmd:   cmd,
		Tty:   false,
	}, nil, nil, nil, "")
	if err != nil {
		panic(err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		panic(err)
	}

	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			panic(err)
		}
	case <-statusCh:
	}

	cout, err := cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStdout: true})
	if err != nil {
		panic(err)
	}

	stdcopy.StdCopy(os.Stdout, os.Stderr, cout)

	return nil
}
