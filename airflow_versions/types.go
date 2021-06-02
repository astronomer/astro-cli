package airflowversions

// Response wraps all houston response structs used for json marashalling
type Response struct {
	AvailableReleases []AirflowVersion `json:"available_releases"`
	Version           string           `json:"version"`
}

// AirflowVersion
type AirflowVersion struct {
	Version     string   `json:"version"`
	Level       string   `json:"level"`
	ReleaseDate string   `json:"release_date"`
	Tags        []string `json:"tags"`
	Channel     string   `json:"channel"`
}
