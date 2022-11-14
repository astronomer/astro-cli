package houston

var (
	appConfig    *AppConfig
	appConfigErr error
)

var (
	AppConfigRequest = queryList{
		{
			version: "0.29.2",
			query: `
			query AppConfig {
				appConfig {
					version
					baseDomain
					byoUpdateRegistryHost
					smtpConfigured
					manualReleaseNames
					configureDagDeployment
					nfsMountDagDeployment
					manualNamespaceNames
					hardDeleteDeployment
					triggererEnabled
					featureFlags
				}
			}`,
		},
		{
			version: "0.28.0",
			query: `
			query AppConfig {
				appConfig {
					version
					baseDomain
					smtpConfigured
					manualReleaseNames
					configureDagDeployment
					nfsMountDagDeployment
					manualNamespaceNames
					hardDeleteDeployment
					triggererEnabled
					featureFlags
				}
			}`,
		},
		{
			version: "0.25.0",
			query: `
			query AppConfig {
				appConfig {
					version
					baseDomain
					smtpConfigured
					manualReleaseNames
					configureDagDeployment
					nfsMountDagDeployment
					manualNamespaceNames
					hardDeleteDeployment
				}
			}`,
		},
	}

	AvailableNamespacesGetRequest = `
	query availableNamespaces {
		availableNamespaces{
			name
		}
	}`

	HoustonVersionQuery = `
	query AppConfig {
		appConfig {
			version
		}
	}`
)

// GetAppConfig - get application configuration
func (h ClientImplementation) GetAppConfig(_ interface{}) (*AppConfig, error) {
	// If application config has already been requested, we do not want to request it again
	// since this is a CLI program that gets executed and exits at the end of execution, we don't want to send multiple
	// times the same call to get the app config, since it probably won't change in a few milliseconds.
	if appConfig != nil || appConfigErr != nil {
		return appConfig, appConfigErr
	}

	reqQuery := AppConfigRequest.GreatestLowerBound(version)
	req := Request{
		Query: reqQuery,
	}

	var r *Response
	r, appConfigErr = req.DoWithClient(h.client)
	if appConfigErr != nil {
		appConfigErr = handleAPIErr(appConfigErr)
		return nil, appConfigErr
	}

	appConfig = r.Data.GetAppConfig
	if appConfig.Flags == (FeatureFlags{}) { // Case when CLI is connected to Houston 0.25.x flags in the response won't be part of featureFlags
		appConfig.Flags.NfsMountDagDeployment = appConfig.NfsMountDagDeployment
		appConfig.Flags.HardDeleteDeployment = appConfig.HardDeleteDeployment
		appConfig.Flags.ManualNamespaceNames = appConfig.ManualNamespaceNames
		appConfig.Flags.TriggererEnabled = appConfig.TriggererEnabled
		appConfig.Flags.TriggererEnabled = appConfig.TriggererEnabled
	}
	return appConfig, nil
}

// GetAvailableNamespace - get namespaces to create deployments
func (h ClientImplementation) GetAvailableNamespaces(_ interface{}) ([]Namespace, error) {
	req := Request{
		Query: AvailableNamespacesGetRequest,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return []Namespace{}, handleAPIErr(err)
	}

	return r.Data.GetDeploymentNamespaces, nil
}

// GetPlatformVersion would fetch the current platform version
func (h ClientImplementation) GetPlatformVersion(_ interface{}) (string, error) {
	if version != "" || versionErr != nil {
		return version, versionErr
	}

	req := Request{
		Query: HoustonVersionQuery,
	}

	r, err := req.DoWithClient(h.client)
	versionErr = err
	if err != nil {
		version = ""
		return "", err
	}
	version = r.Data.GetAppConfig.Version
	return version, nil
}
