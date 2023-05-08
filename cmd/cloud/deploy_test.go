package cloud

import (
	"testing"

	astro "github.com/astronomer/astro-cli/astro-client"
	cloud "github.com/astronomer/astro-cli/cloud/deploy"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestCmdCloudSuite(t *testing.T) {
	suite.Run(t, new(Suite))
}

func execDeployCmd(args ...string) error {
	testUtil.SetupOSArgsForGinkgo()
	cmd := NewDeployCmd()
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return err
}

func (s *Suite) TestDeployImage() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	EnsureProjectDir = func(cmd *cobra.Command, args []string) error {
		return nil
	}

	DeployImage = func(deployInput cloud.InputDeploy, client astro.Client) error {
		return nil
	}

	err := execDeployCmd([]string{"-f"}...)
	s.NoError(err)

	err = execDeployCmd([]string{"test-deployment-id", "--save"}...)
	s.NoError(err)

	err = execDeployCmd([]string{"-f", "test-deployment-id", "--pytest"}...)
	s.NoError(err)

	err = execDeployCmd([]string{"-f", "test-deployment-id", "--parse"}...)
	s.NoError(err)

	err = execDeployCmd([]string{"-f", "test-deployment-id", "--parse", "--pytest"}...)
	s.NoError(err)

	err = execDeployCmd([]string{"test-deployment-id", "--parse", "--pytest"}...)
	s.NoError(err)

	err = execDeployCmd([]string{"test-deployment-id", "--dags"}...)
	s.NoError(err)

	err = execDeployCmd([]string{"-f", "test-deployment-id", "--dags", "--pytest"}...)
	s.NoError(err)

	err = execDeployCmd([]string{"-f", "test-deployment-id", "--dags", "--parse"}...)
	s.NoError(err)

	err = execDeployCmd([]string{"-f", "test-deployment-id", "--dags", "--parse", "--pytest"}...)
	s.NoError(err)
}
