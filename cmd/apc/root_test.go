package apc

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/houston"
	houston_mocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/astronomer/astro-cli/pkg/logger"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

type AddCmdSuite struct {
	suite.Suite
}

func TestAddCmds(t *testing.T) {
	suite.Run(t, new(AddCmdSuite))
}

func (s *AddCmdSuite) TearDownSuite() {
	// Reset the version once this is torn down
	houstonVersion = "0.34.0"
}

func (s *AddCmdSuite) SetupSuite() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
}

var _ suite.TearDownAllSuite = (*AddCmdSuite)(nil)

// Command help must use APC branding: no "software", and no "Astronomer <noun>"
// except the product/resource names below that legitimately keep the prefix.
func (s *AddCmdSuite) TestNoLegacySoftwareBrandingInDescriptions() {
	software := regexp.MustCompile(`(?i)software`)
	astronomerNoun := regexp.MustCompile(`(?i)astronomer\s+(\w+)`)
	kept := map[string]bool{"runtime": true, "deployment": true, "deployments": true, "workspace": true, "workspaces": true}

	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{}, nil)
	houstonMock.On("GetPlatformVersion", nil).Return("0.30.0", nil)

	check := func(where, text string) {
		s.NotRegexp(software, text, "%s contains legacy branding: %q", where, text)
		for _, m := range astronomerNoun.FindAllStringSubmatch(text, -1) {
			noun := strings.ToLower(m[1])
			s.Truef(kept[noun], "%s uses legacy branding %q; rename to APC", where, m[0])
		}
	}

	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, text := range []string{cmd.Short, cmd.Long, cmd.Example} {
			check(cmd.CommandPath(), text)
		}
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			check(fmt.Sprintf("%s --%s", cmd.CommandPath(), f.Name), f.Usage)
		})
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}

	for _, cmd := range AddCmds(houstonMock, new(bytes.Buffer)) {
		walk(cmd)
	}
}

func (s *AddCmdSuite) TestAddCmds() {
	appConfig = &houston.AppConfig{
		TriggererEnabled: true,
		Flags: houston.FeatureFlags{
			TriggererEnabled: true,
		},
	}
	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil)
	houstonMock.On("GetPlatformVersion", nil).Return("0.30.0", nil)
	buf := new(bytes.Buffer)
	cmds := AddCmds(houstonMock, buf)
	for cmdIdx := range cmds {
		s.Contains([]string{"deployment", "deploy [DEPLOYMENT ID]", "user", "workspace", "team"}, cmds[cmdIdx].Use)
	}
	houstonMock.AssertExpectations(s.T())
}

func (s *AddCmdSuite) TestAppConfigFailure() {
	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetAppConfig", mock.Anything).Return(nil, errMock)
	houstonMock.On("GetPlatformVersion", nil).Return("0.30.0", nil)
	buf := new(bytes.Buffer)
	cmds := AddCmds(houstonMock, buf)
	for cmdIdx := range cmds {
		s.Contains([]string{"deployment", "deploy [DEPLOYMENT ID]", "user", "workspace", "team"}, cmds[cmdIdx].Use)
	}
	houstonMock.AssertExpectations(s.T())
	s.Contains(InitDebugLogs, fmt.Sprintf("Error checking feature flag: %s", errMock))
}

func (s *AddCmdSuite) TestPlatformVersionFailure() {
	appConfig = &houston.AppConfig{
		TriggererEnabled: true,
		Flags: houston.FeatureFlags{
			TriggererEnabled: true,
		},
	}
	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil)
	houstonMock.On("GetPlatformVersion", nil).Return("", errMock)
	buf := new(bytes.Buffer)
	cmds := AddCmds(houstonMock, buf)
	for cmdIdx := range cmds {
		s.Contains([]string{"deployment", "deploy [DEPLOYMENT ID]", "user", "workspace", "team"}, cmds[cmdIdx].Use)
	}
	houstonMock.AssertExpectations(s.T())
	s.Contains(InitDebugLogs, fmt.Sprintf("Unable to get Houston version: %s", errMock))
}

func (s *AddCmdSuite) TestSetupLogs() {
	buf := new(bytes.Buffer)
	err := SetUpLogs(buf, "info")
	s.NoError(err)
	s.Equal("info", logger.GetLevel().String())

	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	err = config.CFG.Verbosity.SetHomeString("error")
	s.NoError(err)

	err = SetUpLogs(buf, "warning")
	s.NoError(err)
	s.Equal("error", logger.GetLevel().String())

	err = SetUpLogs(buf, "invalid-level")
	s.EqualError(err, "not a valid logrus Level: \"invalid-level\"")
}

func (s *AddCmdSuite) TestPrintDebugLogs() {
	buf := new(bytes.Buffer)
	err := SetUpLogs(buf, "debug")
	s.NoError(err)

	InitDebugLogs = []string{"test log line"}

	PrintDebugLogs()
	s.Nil(InitDebugLogs)
	s.Contains(buf.String(), "test log line")
}
