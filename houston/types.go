package houston

// HoustonReasponse wraps all houston response structs used for json marashalling
type HoustonResponse struct {
	Data struct {
		CreateDeployment *StatusResponse `json:"createDeployment,omitempty"`
		CreateToken      *Token          `json:"createToken,omitempty"`
		FetchDeployments []Deployment    `json:"fetchDeployments"`
	} `json:"data"`
}

// CreateTokenResponse defines structure of a houston response CreateTokenResponse object
type CreateTokenResponse struct {
	Data struct {
		CreateToken Token `json:"createToken"`
	} `json:"data"`
}

// FetchDeploymentsResponse defines structure of a houston response FetchDeploymentsResponse object
type FetchDeploymentsResponse struct {
	Data struct {
		FetchDeployments []Deployment `json:"fetchDeployments"`
	} `json:"data"`
}

// Token defines structure of a houston response token object
type Token struct {
	Success bool    `json:"success"`
	Message string  `json:"message"`
	Token   string  `json:"token"`
	Decoded Decoded `json:"decoded"`
}

// StatusResponse defines structure of a houston response StatusResponse object
type StatusResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Code    string `json:"code"`
	Id      string `json:"id"`
}

// Deployment defines structure of a houston response Deployment object
type Deployment struct {
	Id          string `json:"uuid"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	ReleaseName string `json:"release_name"`
	Version     string `json:"version"`
}

// Decoded defines structure of a houston response Decoded object
type Decoded struct {
	ID  string `json:"id"`
	SU  bool   `json:"sU"`
	Iat int    `json:"iat"`
	Exp int    `json:"exp"`
}
