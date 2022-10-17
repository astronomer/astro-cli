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

	httpClient := &http.Client{}
	req, err := http.NewRequest(method, url, http.NoBody)
	if err != nil {
		fmt.Println(err)
		return ""
	}
	res, err := httpClient.Do(req)
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

	astroSQLCliVersion := getPypiVersion()
	opts := types.ImageBuildOptions{
		Dockerfile: "Dockerfile.sql_cli",
		Tags:       []string{"sql_cli"},
	}

	if astroSQLCliVersion != "" {
		opts.BuildArgs = map[string]*string{"VERSION": &astroSQLCliVersion}
	}

	_, currentfilePath, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Println("Failed to get current file path and hence cannot locate Dockerfile for building the image")
	}
	dockerfilePath := path.Join(path.Dir(currentfilePath), "../Dockerfile.sql_cli")
	fmt.Println("Building image...")
	body, err := cli.ImageBuild(ctx, getContext(dockerfilePath), opts)
	if err != nil {
		fmt.Printf("Image building failed %v ", err)
	}
	buf := new(strings.Builder)
	_, err = io.Copy(buf, body.Body)
	if err != nil {
		fmt.Printf("Image build response read failed %s", err)
	}

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

	_, err = stdcopy.StdCopy(os.Stdout, os.Stderr, cout)

	if err != nil {
		panic(err)
	}

	return nil
}
