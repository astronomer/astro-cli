package sql

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"strings"

	"github.com/astronomer/astro-cli/sql/include"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/jsonmessage"
)

const (
	astroSQLCLIProjectURL     = "https://pypi.org/pypi/astro-sql-cli/json"
	astroSQLCLIConfigURL      = "https://raw.githubusercontent.com/astronomer/astro-sdk/astro-cli/sql-cli/config/astro-cli.json"
	SQLCliDockerfilePath      = ".Dockerfile.sql_cli"
	SQLCLIDockerfileWriteMode = 0o600
	SQLCliDockerImageName     = "sql_cli"
	PythonVersion             = "3.9"
)

var (
	Docker          = NewDockerBind
	Io              = NewIoBind
	DisplayMessages = displayMessages
	Os              = NewOsBind
)

func getContext(filePath string) io.Reader {
	ctx, _ := archive.TarWithOptions(filePath, &archive.TarOptions{})
	return ctx
}

func displayMessages(r io.Reader) error {
	decoder := json.NewDecoder(r)
	var prevMessage jsonmessage.JSONMessage
	isFirstMessage := true
	for {
		var jsonMessage jsonmessage.JSONMessage
		if err := decoder.Decode(&jsonMessage); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if jsonMessage.Stream == "\n" {
			continue
		}
		if jsonMessage.Error != nil {
			return jsonMessage.Error
		}
		// We only print steps which are actually running, e.g.
		// Step 2/4 : ENV ASTRO_CLI Yes
		//  ---> Running in 0afb2e0c5ad7
		if strings.HasPrefix(prevMessage.Stream, "Step ") && strings.HasPrefix(jsonMessage.Stream, " ---> Running in ") {
			if isFirstMessage {
				fmt.Println("Installing flow... This might take some time.")
				isFirstMessage = false
			}
			err := prevMessage.Display(os.Stdout, true)
			fmt.Println()
			if err != nil {
				return err
			}
		}
		prevMessage = jsonMessage
	}
	return nil
}

func ConvertReadCloserToString(readCloser io.ReadCloser) (string, error) {
	buf := new(strings.Builder)
	_, err := Io().Copy(buf, readCloser)
	if err != nil {
		return "", fmt.Errorf("converting readcloser output to string failed %w", err)
	}
	return buf.String(), nil
}

func CommonDockerUtil(cmd, args []string, flags map[string]string, mountDirs []string, returnOutput bool) (exitCode int64, output io.ReadCloser, err error) {
	var statusCode int64
	var cout io.ReadCloser

	ctx := context.Background()

	cli, err := Docker()
	if err != nil {
		return statusCode, cout, fmt.Errorf("docker client initialization failed %w", err)
	}

	astroSQLCliVersion, err := getPypiVersion(astroSQLCLIProjectURL)
	if err != nil {
		return statusCode, cout, err
	}

	baseImage, err := getBaseDockerImageURI(astroSQLCLIConfigURL)
	if err != nil {
		fmt.Println(err)
	}

	currentUser, _ := user.Current()

	dockerfileContent := []byte(fmt.Sprintf(include.Dockerfile, baseImage, astroSQLCliVersion, currentUser.Uid, currentUser.Username))
	if err := Os().WriteFile(SQLCliDockerfilePath, dockerfileContent, SQLCLIDockerfileWriteMode); err != nil {
		return statusCode, cout, fmt.Errorf("error writing dockerfile %w", err)
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
		return statusCode, cout, fmt.Errorf("image building failed %w", err)
	}

	if err := DisplayMessages(body.Body); err != nil {
		return statusCode, cout, fmt.Errorf("image build response read failed %w", err)
	}

	cmd = append(cmd, args...)
	for key, value := range flags {
		cmd = append(cmd, fmt.Sprintf("--%s", key), value)
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
			User:  fmt.Sprintf("%s:%s", currentUser.Uid, currentUser.Gid),
		},
		&container.HostConfig{
			Binds: binds,
		},
		nil,
		nil,
		"",
	)
	if err != nil {
		return statusCode, cout, fmt.Errorf("docker container creation failed %w", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return statusCode, cout, fmt.Errorf("docker container start failed %w", err)
	}

	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return statusCode, cout, fmt.Errorf("docker container wait failed %w", err)
		}
	case status := <-statusCh:
		statusCode = status.StatusCode
	}

	cout, err = cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return statusCode, cout, fmt.Errorf("docker container logs fetching failed %w", err)
	}

	if !returnOutput {
		if _, err := Io().Copy(os.Stdout, cout); err != nil {
			return statusCode, cout, fmt.Errorf("docker logs forwarding failed %w", err)
		}
	}

	if err := cli.ContainerRemove(ctx, resp.ID, types.ContainerRemoveOptions{}); err != nil {
		return statusCode, cout, fmt.Errorf("docker remove failed %w", err)
	}

	return statusCode, cout, nil
}
