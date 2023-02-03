package sql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"regexp"
	"strings"

	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/astronomer/astro-cli/sql/include"
	"github.com/astronomer/astro-cli/version"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/jsonmessage"
)

const (
	astroSQLCLIProjectURL     = "https://pypi.org/pypi/astro-sql-cli/json"
	astroSQLCLIConfigURL      = "https://raw.githubusercontent.com/astronomer/astro-sdk/1673-minor-version-match/sql-cli/config/astro-cli.json"
	sqlCLIDockerfilePath      = ".Dockerfile.sql_cli"
	fileWriteMode             = 0o600
	sqlCLIDockerImageName     = "sql_cli"
	astroDockerfilePath       = "Dockerfile"
	astroRequirementsfilePath = "requirements.txt"
	runtimeImagePrefix        = "quay.io/astronomer/astro-runtime:"
	two                       = 2
)

var (
	Docker                     = NewDockerBind
	Io                         = NewIoBind
	DisplayMessages            = OriginalDisplayMessages
	Os                         = NewOsBind
	BufIo                      = NewBufIOBind
	ErrNoBaseAstroRuntimeImage = errors.New("base image is not an Astro runtime image in the provided Dockerfile")
	ErrPythonSDKVersionNotMet  = errors.New("required version for Python SDK dependency not met")
	ErrVersionExtraction       = errors.New("extracting version from image failed")
	astroRuntimeVersionRegex   = regexp.MustCompile(runtimeImagePrefix + "([^-]*)")
)

func getContext(filePath string) io.Reader {
	ctx, _ := archive.TarWithOptions(filePath, &archive.TarOptions{})
	return ctx
}

func OriginalDisplayMessages(r io.Reader) error {
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

var ConvertReadCloserToString = func(readCloser io.ReadCloser) (string, error) {
	buf := new(strings.Builder)
	_, err := Io().Copy(buf, readCloser)
	if err != nil {
		return "", fmt.Errorf("converting readcloser output to string failed %w", err)
	}
	return buf.String(), nil
}

var ExecuteCmdInDocker = func(cmd, mountDirs []string, returnOutput bool) (exitCode int64, output io.ReadCloser, err error) {
	var statusCode int64
	var cout io.ReadCloser

	ctx := context.Background()

	cli, err := Docker()
	if err != nil {
		return statusCode, cout, fmt.Errorf("docker client initialization failed %w", err)
	}

	baseImage, err := getBaseDockerImageURI(astroSQLCLIConfigURL)
	if err != nil {
		fmt.Println(err)
	}
	astroSQLCliVersion, err := getPypiVersion(astroSQLCLIConfigURL, version.CurrVersion)
	if err != nil {
		return statusCode, cout, err
	}
	preReleaseOptString := ""
	if astroSQLCliVersion.Prerelease {
		preReleaseOptString = "--pre"
	}

	currentUser, _ := user.Current()

	dockerfileContent := []byte(fmt.Sprintf(include.Dockerfile, baseImage, astroSQLCliVersion.Version, preReleaseOptString, currentUser.Username, currentUser.Uid, currentUser.Username))
	if err := Os().WriteFile(sqlCLIDockerfilePath, dockerfileContent, fileWriteMode); err != nil {
		return statusCode, cout, fmt.Errorf("error writing dockerfile %w", err)
	}
	defer os.Remove(sqlCLIDockerfilePath)

	body, err := cli.ImageBuild(
		ctx,
		getContext(sqlCLIDockerfilePath),
		&types.ImageBuildOptions{
			Dockerfile: sqlCLIDockerfilePath,
			Tags:       []string{sqlCLIDockerImageName},
		},
	)
	if err != nil {
		return statusCode, cout, fmt.Errorf("image building failed %w", err)
	}

	if err := DisplayMessages(body.Body); err != nil {
		return statusCode, cout, fmt.Errorf("image build response read failed %w", err)
	}

	binds := []string{}
	for _, mountDir := range mountDirs {
		binds = append(binds, fmt.Sprintf("%s:%s", mountDir, mountDir))
	}

	resp, err := cli.ContainerCreate(
		ctx,
		&container.Config{
			Image: sqlCLIDockerImageName,
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

var getAstroDockerfileRuntimeVersion = func() (string, error) {
	file, err := Os().Open(astroDockerfilePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := BufIo().NewScanner(file)
	scanner.Scan()
	text := scanner.Text()
	if !strings.Contains(text, runtimeImagePrefix) {
		return "", ErrNoBaseAstroRuntimeImage
	}

	stringSubMatch := astroRuntimeVersionRegex.FindStringSubmatch(text)
	if len(stringSubMatch) < two {
		return "", ErrVersionExtraction
	}
	runtimeVersion := astroRuntimeVersionRegex.FindStringSubmatch(text)[1]

	return runtimeVersion, nil
}

var EnsurePythonSdkVersionIsMet = func(promptRunner input.PromptRunner, installedSQLCLIVersion string) error {
	astroRuntimeVersion, err := getAstroDockerfileRuntimeVersion()
	if err != nil {
		return err
	}

	requiredRuntimeVersion, requiredPythonSDKVersion, err := getPythonSDKComptability(astroSQLCLIConfigURL, installedSQLCLIVersion)
	if err != nil {
		return err
	}

	runtimeVersionMet, err := util.IsRequiredVersionMet(astroRuntimeVersion, requiredRuntimeVersion)
	if err != nil {
		return err
	}

	requiredPythonSDKDependency := "\nastro-sdk-python" + requiredPythonSDKVersion
	b, err := Os().ReadFile(astroRequirementsfilePath)
	if err != nil {
		return err
	}
	existingRequirements := string(b)

	if !runtimeVersionMet && !strings.Contains(existingRequirements, requiredPythonSDKDependency) {
		result, err := input.PromptGetConfirmation(promptRunner)
		if err != nil {
			return err
		}
		if !result {
			return ErrPythonSDKVersionNotMet
		}

		f, err := Os().OpenFile(astroRequirementsfilePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, fileWriteMode)
		if err != nil {
			return err
		}

		defer f.Close()
		if _, err = f.WriteString(requiredPythonSDKDependency); err != nil {
			return err
		}
	}
	return nil
}
