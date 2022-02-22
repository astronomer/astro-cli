package houston

var (
	appConfig    *AppConfig
	appConfigErr error
)

// GetAppConfig - get application configuration
func (h ClientImplementation) GetAppConfig() (*AppConfig, error) {
	// If application config has already been requested, we do not want to request it again
	// since this is a CLI program that gets executed and exits at the end of execution, we don't want to send multiple
	// times the same call to get the app config, since it probably won't change in a few milliseconds.
	if appConfig != nil || appConfigErr != nil {
		return appConfig, appConfigErr
	}

	req := Request{
		Query: AppConfigRequest,
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
	req := Request{
		Query: AvailableNamespacesGetRequest,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return []Namespace{}, handleAPIErr(err)
	}

	return r.Data.GetDeploymentNamespaces, nil
}
