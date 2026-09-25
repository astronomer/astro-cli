package settings

// setHTTPClient swaps the client Airflow API calls go through and returns the
// previous one, so a test can point them at an httptest server and restore it.
func setHTTPClient(c HTTPDoer) HTTPDoer {
	prev := httpClient
	httpClient = c
	return prev
}
