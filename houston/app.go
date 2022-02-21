package houston

// GetAppConfig - get application configuration
func (h ClientImplementation) GetAppConfig() (*AppConfig, error) {
	req := Request{
		Query: AppConfigRequest,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.GetAppConfig, nil
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
