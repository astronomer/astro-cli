package sql

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

func getPypiVersion() string {

	url := "https://pypi.org/pypi/astro-sql-cli/json"
	method := "GET"

	client := &http.Client{}
	req, err := http.NewRequest(method, url, nil)

	if err != nil {
		fmt.Println(err)
		return ""
	}
	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return ""
	}
	defer res.Body.Close()

	type response struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	var resp response
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		fmt.Println(err)
		return ""
	}

	return resp.Info.Version
}

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

	astro_sql_cli_version := getPypiVersion()
	opts := types.ImageBuildOptions{
		Dockerfile: "Dockerfile.sql_cli",
		Tags:       []string{"sql_cli"},
	}

	if astro_sql_cli_version != "" {
		opts.BuildArgs = map[string]*string{"VERSION": &astro_sql_cli_version}
	}

	_, currentfilePath, _, _ := runtime.Caller(0)
	dockerfilePath := path.Join(path.Dir(currentfilePath), "../Dockerfile.sql_cli")
	fmt.Println("Building image...")
	body, err := cli.ImageBuild(ctx, getContext(dockerfilePath), opts)
	buf := new(strings.Builder)
	_, err = io.Copy(buf, body.Body)

	for key, value := range flags {
		cmd = append(cmd, []string{fmt.Sprintf("--%s", key), value}...)
	}
	fmt.Println(cmd)
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: "sql_cli",
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
