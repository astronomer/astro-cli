package sql

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/astronomer/astro-cli/sql/include"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/jsonmessage"
)

const (
	SQLCliDockerfilePath      = ".Dockerfile.sql_cli"
	SQLCLIDockerfileWriteMode = 0o600
	SQLCliDockerImageName     = "sql_cli"
	PythonVersion             = "3.9"
)

var (
	DockerClientInit   = NewDockerClient
	IoCopy             = io.Copy
	PrintBuildingSteps = printBuildingSteps
	Println            = fmt.Println
)

func getContext(filePath string) io.Reader {
	ctx, _ := archive.TarWithOptions(filePath, &archive.TarOptions{})
	return ctx
}

func printBuildingSteps(r io.Reader) error {
	decoder := json.NewDecoder(r)
	var prevStream string
	var currStream string
	firstMessage := true
	for {
		var jsonMessage jsonmessage.JSONMessage
		if err := decoder.Decode(&jsonMessage); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if err := jsonMessage.Error; err != nil {
			return err
		}
		if jsonMessage.Stream == "\n" {
			continue
		}
		currStream = jsonMessage.Stream
		// We only print steps which are actually running, e.g.
		// Step 2/4 : ENV ASTRO_CLI Yes
		//  ---> Running in 0afb2e0c5ad7
		if strings.HasPrefix(prevStream, "Step ") && strings.HasPrefix(currStream, " ---> Running in ") {
			if firstMessage {
				if _, err := Println("Installing flow.. This might take some time."); err != nil {
					return err
				}
				firstMessage = false
			}
			if _, err := Println(prevStream); err != nil {
				return err
			}
		}
		prevStream = currStream
	}
	return nil
}

func CommonDockerUtil(cmd, args []string, flags map[string]string, mountDirs []string) error {
	ctx := context.Background()

	cli, err := DockerClientInit()
	if err != nil {
		return fmt.Errorf("docker client initialization failed %w", err)
	}

	astroSQLCliVersion, err := getPypiVersion(astroSQLCliProjectURL)
	if err != nil {
		return err
	}

	dockerfileContent := []byte(fmt.Sprintf(include.Dockerfile, PythonVersion, astroSQLCliVersion))
	if err := os.WriteFile(SQLCliDockerfilePath, dockerfileContent, SQLCLIDockerfileWriteMode); err != nil {
		return fmt.Errorf("error writing dockerfile %w", err)
	}
	defer os.Remove(SQLCliDockerfilePath)

	body, err := cli.ImageBuild(
		ctx,
		getContext(SQLCliDockerfilePath),
		&types.ImageBuildOptions{
			Dockerfile: SQLCliDockerfilePath,
			Tags:       []string{SQLCliDockerImageName},
		},
	)
	if err != nil {
		return fmt.Errorf("image building failed %w", err)
	}

	if err := PrintBuildingSteps(body.Body); err != nil {
		return fmt.Errorf("image build response read failed %w", err)
	}

	cmd = append(cmd, args...)
	for key, value := range flags {
		cmd = append(cmd, fmt.Sprintf("--%s %s", key, value))
	}

	binds := []string{}
	for _, mountDir := range mountDirs {
		binds = append(binds, fmt.Sprintf("%s:%s", mountDir, mountDir))
	}

	resp, err := cli.ContainerCreate(
		ctx,
		&container.Config{
			Image: SQLCliDockerImageName,
			Cmd:   cmd,
			Tty:   true,
		},
		&container.HostConfig{
			Binds: binds,
		},
		nil,
		nil,
		"",
	)
	if err != nil {
		return fmt.Errorf("docker container creation failed %w", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("docker container start failed %w", err)
	}

	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("docker container wait failed %w", err)
		}
	case <-statusCh:
	}

	cout, err := cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return fmt.Errorf("docker container logs fetching failed %w", err)
	}

	if _, err := IoCopy(os.Stdout, cout); err != nil {
		return fmt.Errorf("docker logs forwarding failed %w", err)
	}

	return nil
}
