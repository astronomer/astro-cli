package sql

import (
	"context"
	"io"
	"os"
	"path"
	"runtime"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/spf13/cobra"
)

func GetContext(filePath string) io.Reader {
	ctx, _ := archive.TarWithOptions(filePath, &archive.TarOptions{})
	return ctx
}

func Flow(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	opts := types.ImageBuildOptions{
		Dockerfile: "Dockerfile.sql_cli",
		Tags:       []string{"sql_cli"},
		NoCache:    true,
	}

	_, currentfilePath, _, _ := runtime.Caller(0)
	dockerfilePath := path.Join(path.Dir(currentfilePath), "../Dockerfile.sql_cli")
	_, err = cli.ImageBuild(ctx, GetContext(dockerfilePath), opts)
	if err != nil {
		panic(err)
	}

	args = append([]string{"flow"}, args...)
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: "sql_cli",
		Cmd:   args,
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
