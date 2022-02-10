package deployment

import (
	"errors"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/stretchr/testify/assert"
)

func TestGetAppConfig(t *testing.T) {
	mockAppConfig := &houston.AppConfig{
		Version:                "1.0.5",
		BaseDomain:             "localdev.me",
		SMTPConfigured:         false,
		ManualReleaseNames:     false,
		ConfigureDagDeployment: false,
		NfsMountDagDeployment:  false,
		HardDeleteDeployment:   false,
		ManualNamespaceNames:   true,
		TriggererEnabled:       true,
		Flags: houston.FeatureFlags{
			TriggererEnabled:     true,
			ManualNamespaceNames: true,
		},
	}
	mockError := errors.New("api error") //nolint:goerr113

	t.Run("success", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		// once to check that the houston client is only called once
		api.On("GetAppConfig").Return(mockAppConfig, nil).Once()

		config, err := GetAppConfig(api)
		assert.NoError(t, err)
		assert.Equal(t, config, mockAppConfig)

		config, err = GetAppConfig(api)
		assert.NoError(t, err)
		assert.Equal(t, config, mockAppConfig)

		api.AssertExpectations(t)
	})

	t.Run("error", func(t *testing.T) {
		// reset the local variables
		appConfig = nil
		appConfigErr = nil

		api := new(mocks.ClientInterface)
		// once to check that the houston client is only called once
		api.On("GetAppConfig").Return(nil, mockError).Once()

		config, err := GetAppConfig(api)
		assert.EqualError(t, err, mockError.Error())
		assert.Nil(t, config)

		config, err = GetAppConfig(api)
		assert.EqualError(t, err, mockError.Error())
		assert.Nil(t, config)

		api.AssertExpectations(t)
	})
}
