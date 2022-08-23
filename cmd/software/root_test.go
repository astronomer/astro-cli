package software

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/houston"
	houston_mocks "github.com/astronomer/astro-cli/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestAddCmds(t *testing.T) {
	appConfig := &houston.AppConfig{
		TriggererEnabled: true,
		Flags: houston.FeatureFlags{
			TriggererEnabled: true,
		},
	}
	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetAppConfig", nil).Return(appConfig, nil)
	buf := new(bytes.Buffer)
	cmds := AddCmds(houstonMock, buf)
	for cmdIdx := range cmds {
		assert.Contains(t, []string{"deployment", "deploy [DEPLOYMENT ID]", "user", "workspace", "team"}, cmds[cmdIdx].Use)
	}
	houstonMock.AssertExpectations(t)
}

func TestAppConfigFailure(t *testing.T) {
	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetAppConfig", nil).Return(nil, errMock)
	buf := new(bytes.Buffer)
	cmds := AddCmds(houstonMock, buf)
	for cmdIdx := range cmds {
		assert.Contains(t, []string{"deployment", "deploy [DEPLOYMENT ID]", "user", "workspace", "team"}, cmds[cmdIdx].Use)
	}
	houstonMock.AssertExpectations(t)
	assert.Contains(t, initDebugLogs, fmt.Sprintf("Error checking feature flag: %s", errMock))
}

func TestSetupLogs(t *testing.T) {
	buf := new(bytes.Buffer)
	err := SetUpLogs(buf, "info")
	assert.NoError(t, err)
	assert.Equal(t, "info", logrus.GetLevel().String())

	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	err = config.CFG.Verbosity.SetHomeString("error")
	assert.NoError(t, err)

	err = SetUpLogs(buf, "warning")
	assert.NoError(t, err)
	assert.Equal(t, "error", logrus.GetLevel().String())

	err = SetUpLogs(buf, "invalid-level")
	assert.EqualError(t, err, "not a valid logrus Level: \"invalid-level\"")
}

func TestPrintDebugLogs(t *testing.T) {
	buf := new(bytes.Buffer)
	err := SetUpLogs(buf, "debug")
	assert.NoError(t, err)

	initDebugLogs = []string{"test log line"}

	PrintDebugLogs()
	assert.Nil(t, initDebugLogs)
	assert.Contains(t, buf.String(), "test log line")
}
