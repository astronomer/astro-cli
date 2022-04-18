package houston

func (h ClientImplementation) GetRuntimeReleases(airflowVersion string) (RuntimeReleases, error) {
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
