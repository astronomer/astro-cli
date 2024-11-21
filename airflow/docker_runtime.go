package airflow

import "github.com/astronomer/astro-cli/cmd/utils"

type DockerRuntime struct{}

func (p DockerRuntime) Initialize() error {
	if utils.IsMac() {
		if err := startDocker(); err != nil {
			return err
		}
	}
	return nil
}

func (p DockerRuntime) Configure() error {
	return nil
}

func (p DockerRuntime) ConfigureOrKill() error {
	return nil
}

func (p DockerRuntime) Kill() error {
	return nil
}
