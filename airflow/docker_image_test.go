package airflow

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/astronomer/astro-cli/airflow/types"
	"github.com/stretchr/testify/assert"
)

func TestBuild(t *testing.T) {
	handler := DockerImage{
		imageName: "testing",
	}

	cwd, err := os.Getwd()
	assert.NoError(t, err)

	options := types.ImageBuildConfig{
		Path:    cwd,
		NoCache: false,
	}

	previousDockerExec := dockerExec

	t.Run("build success", func(t *testing.T) {
		dockerExec = func(stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		err = handler.Build(options)
		assert.NoError(t, err)
	})

	t.Run("build --no-cache", func(t *testing.T) {
		options.NoCache = true
		dockerExec = func(stdout, stderr io.Writer, args ...string) error {
			assert.Contains(t, args, "--no-cache")
			return nil
		}
		err = handler.Build(options)
		assert.NoError(t, err)
	})

	t.Run("build error", func(t *testing.T) {
		mockError := errors.New("build error") //nolint:goerr113
		dockerExec = func(stdout, stderr io.Writer, args ...string) error {
			return mockError
		}
		err = handler.Build(options)
		assert.Contains(t, err.Error(), mockError.Error())
	})

	dockerExec = previousDockerExec
}
