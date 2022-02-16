package version

import (
	"bytes"
	"testing"

	"github.com/astronomer/astro-cli/houston"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	mocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/stretchr/testify/assert"
)

func TestValidateCompatibility(t *testing.T) {
	testUtil.InitTestConfig()

	t.Run("version mismatched", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig").Return(mockAppConfig, nil)
		output := new(bytes.Buffer)
		cliVer := "0.19.1"
		err := ValidateCompatibility(api, output, cliVer, false)
		assert.NoError(t, err)
		// check that there is no output because version matched
		assert.Equal(t, output, &bytes.Buffer{})
	})

	t.Run("missing cli version", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig").Return(mockAppConfig, nil)

		output := new(bytes.Buffer)
		cliVer := ""
		err := ValidateCompatibility(api, output, cliVer, false)
		assert.NoError(t, err)
		// check that there is no output because cli version is missing
		assert.Equal(t, output, &bytes.Buffer{})
		api.AssertExpectations(t)
	})

	t.Run("cli downgrade", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig").Return(mockAppConfig, nil)

		output := new(bytes.Buffer)
		cliVer := "0.20.1"
		err := ValidateCompatibility(api, output, cliVer, false)
		assert.NoError(t, err)
		expected := "Your Astro CLI Version (0.20.1) is ahead of the server version (0.19.1).\nConsider downgrading your Astro CLI to match. See https://www.astronomer.io/docs/cli-quickstart for more information.\n"
		// check that user can see correct message
		assert.Equal(t, expected, output.String())
		api.AssertExpectations(t)
	})

	t.Run("cli upgrade", func(t *testing.T) {
		appConfig := *mockAppConfig
		appConfig.Version = "1.0.0"
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig").Return(&appConfig, nil)

		output := new(bytes.Buffer)
		cliVer := "0.17.1"
		err := ValidateCompatibility(api, output, cliVer, false)
		assert.Error(t, err)
		expected := "There is an update for Astro CLI. You're using version 0.17.1, but 1.0.0 is the server version.\nPlease upgrade to the matching version before continuing. See https://www.astronomer.io/docs/cli-quickstart for more information.\nTo skip this check use the --skip-version-check flag.\n"
		// check that user can see correct message
		assert.EqualError(t, err, expected)
		api.AssertExpectations(t)
	})

	t.Run("version bypass", func(t *testing.T) {
		api := new(mocks.ClientInterface)

		output := new(bytes.Buffer)
		cliVer := "0.17.1"
		err := ValidateCompatibility(api, output, cliVer, true)
		expected := ""

		assert.NoError(t, err)
		// check that user can bypass major version check
		assert.Equal(t, expected, output.String())
	})

	t.Run("minor warning", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig").Return(mockAppConfig, nil)

		output := new(bytes.Buffer)
		cliVer := "0.18.1"
		err := ValidateCompatibility(api, output, cliVer, false)
		assert.NoError(t, err)
		expected := "A new minor version of Astro CLI is available. Your version is 0.18.1 and 0.19.1 is the latest.\nSee https://www.astronomer.io/docs/cli-quickstart for more information.\n"
		// check that user can see correct warning message
		assert.Equal(t, expected, output.String())
		api.AssertExpectations(t)
	})

	t.Run("client failure", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig").Return(nil, errMock)

		output := new(bytes.Buffer)
		cliVer := "0.15.1"
		err := ValidateCompatibility(api, output, cliVer, false)
		assert.Error(t, err)
		api.AssertExpectations(t)
	})

	t.Run("get config version mismatch", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig").Return(nil, houston.ErrFieldsNotAvailable{})

		output := new(bytes.Buffer)
		err := ValidateCompatibility(api, output, "0.15.1", false)
		assert.EqualError(t, err, ErrVersionMismatch{}.Error())
		api.AssertExpectations(t)
	})
}

func TestCompareVersionsInvalidServerVer(t *testing.T) {
	output := new(bytes.Buffer)
	err := compareVersions("INVALID VERSION", "0.17.1", output)
	assert.Error(t, err)
}

func TestCompareVersionsInvalidCliVer(t *testing.T) {
	output := new(bytes.Buffer)
	err := compareVersions("0.17.1", "INVALID VERSION", output)
	assert.Error(t, err)
}

func TestParseVersion(t *testing.T) {
	ver, err := parseVersion("0.17.1")
	assert.NoError(t, err)
	if assert.NotNil(t, ver) {
		assert.Equal(t, uint64(0), ver.Major())
		assert.Equal(t, uint64(17), ver.Minor())
	}
}
