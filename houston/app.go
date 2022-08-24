package houston

var (
	appConfig    *AppConfig
	appConfigErr error
	version      string
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
func (h ClientImplementation) GetAppConfig() (*AppConfig, error) {
	if err := h.ValidateAvailability(); err != nil {
		return nil, err
	}

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
	return appConfig, nil
}

// GetAvailableNamespace - get namespaces to create deployments
func (h ClientImplementation) GetAvailableNamespaces() ([]Namespace, error) {
	if err := h.ValidateAvailability(); err != nil {
		return []Namespace{}, err
	}

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
// This method bypass method availability check as this is to be called by that logic itself
func (h ClientImplementation) GetPlatformVersion() (string, error) {
	if version != "" {
		return version, nil
	}

	req := Request{
		Query: HoustonVersionQuery,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return "", err
	}
	version = r.Data.GetAppConfig.Version
	return version, nil
}
