package houston

// HoustonReasponse wraps all houston response structs used for json marashalling
type HoustonResponse struct {
	Data struct {
		CreateDeployment *Status      `json:"createDeployment,omitempty"`
		CreateToken      *Token       `json:"createToken,omitempty"`
		FetchDeployments []Deployment `json:"fetchDeployments,omitempty"`
		CreateUser       *Token       `json:"createUser,omitempty"`
	} `json:"data"`
	Errors []Error `json:"errors,omitempty"`
}

// Token defines structure of a houston response token object
type Token struct {
	Success bool    `json:"success"`
	Message string  `json:"message"`
	Token   string  `json:"token"`
	Decoded Decoded `json:"decoded"`
}

// Status defines structure of a houston response StatusResponse object
type Status struct {
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

// Error defines struct of a houston response Error object
type Error struct {
	Message string `json:"message"`
}

// Decoded defines structure of a houston response Decoded object
type Decoded struct {
	ID  string `json:"id"`
	SU  bool   `json:"sU"`
	Iat int    `json:"iat"`
	Exp int    `json:"exp"`
}
