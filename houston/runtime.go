package houston

var GetRuntimeReleases = `
	query runtimeReleases($airflowVersion: String, $clusterId: Uuid) {
		runtimeReleases(airflowVersion: $airflowVersion, clusterId: $clusterId) {
			version
			airflowVersion
			airflowDatabaseMigrations
		}
	}`

func (h ClientImplementation) GetRuntimeReleases(vars map[string]interface{}) (RuntimeReleases, error) {
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
