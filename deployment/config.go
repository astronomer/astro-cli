package deployment

import "github.com/astronomer/astro-cli/houston"

var (
	appConfig    *houston.AppConfig
	appConfigErr error
)

var GetAppConfig = func(client houston.ClientInterface) (*houston.AppConfig, error) {
	// If application config has already been requested, we do not want to request it again
	// since this is a CLI program that gets executed and exits at the end of execution, we don't want to send multiple
	// times the same call to get the app config, since it probably won't change in a few milliseconds.
	if appConfig == nil && appConfigErr == nil {
		appConfig, appConfigErr = client.GetAppConfig()
	}

	return appConfig, appConfigErr
}
