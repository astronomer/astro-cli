package houston

var GetRuntimeReleases = `
	query runtimeReleases($airflowVersion: String) {
		runtimeReleases(airflowVersion: $airflowVersion) {
			version
			airflowVersion
			airflowDatabaseMigrations
		}
	}`

func (h ClientImplementation) GetRuntimeReleases(airflowVersion string) (RuntimeReleases, error) {
	if err := h.ValidateAvailability(); err != nil {
		return RuntimeReleases{}, err
	}

	vars := make(map[string]interface{})
	if airflowVersion != "" {
		vars["airflowVersion"] = airflowVersion
	}
	dReq := Request{
		Query:     GetRuntimeReleases,
		Variables: vars,
	}

	resp, err := dReq.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return resp.Data.RuntimeReleases, nil
}
