package houston

import (
	"errors"

	"github.com/astronomer/astro-cli/config"
)

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
		{
			version: "1.0.0",
			query: `
			query AppConfig($clusterId: Uuid) {
				appConfig(clusterId: $clusterId) {
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
	}

	AvailableNamespacesGetRequest = queryList{
		{
			version: "0.25.0",
			query: `
			query availableNamespaces {
				availableNamespaces {
					name
				}
			}`,
		},
		{
			version: "1.0.0",
			query: `
			query availableNamespaces($clusterId: Uuid!) {
				availableNamespaces(clusterId: $clusterId) {
					name
				}
			}`,
		},
	}

	HoustonVersionQuery = `
	query AppConfig {
		appConfig {
			version
		}
	}`
)

// GetAppConfig - get application configuration
func (h ClientImplementation) GetAppConfig(clusterID string) (*AppConfig, error) {
	// Even if application config has already been requested, we want to request it again
	// Because we want to get the app config for the clusterID if it is provided
	vars := map[string]interface{}{}
	if clusterID != "" {
		vars["clusterId"] = clusterID
	}
	reqQuery := AppConfigRequest.GreatestLowerBound(version)
	req := Request{
		Query:     reqQuery,
		Variables: vars,
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
func (h ClientImplementation) GetAvailableNamespaces(vars map[string]interface{}) ([]Namespace, error) {
	reqVars := map[string]interface{}{}
	if vars["clusterID"] != "" && vars["clusterID"] != nil {
		reqVars["clusterId"] = vars["clusterID"]
	}
	req := Request{
		Query:     AvailableNamespacesGetRequest.GreatestLowerBound(version),
		Variables: reqVars,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return []Namespace{}, handleAPIErr(err)
	}

	return r.Data.GetDeploymentNamespaces, nil
}

// GetPlatformVersion would fetch the current platform version
func (h ClientImplementation) GetPlatformVersion(_ interface{}) (string, error) {
	// we would like to retry on ErrGetHomeString error in case context has been set correctly now
	if version != "" || (versionErr != nil && !errors.Is(versionErr, config.ErrGetHomeString)) {
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
