package houston

// HoustonResponse wraps all houston response structs used for json marashalling
type HoustonResponse struct {
	Data struct {
		CreateDeployment *Status      `json:"createDeployment,omitempty"`
		CreateToken      *AuthUser    `json:"createToken,omitempty"`
		CreateUser       *Token       `json:"createUser,omitempty"`
		CreateWorkspace  *Workspace   `json:"createTeam,omitempty"`
		FetchDeployments []Deployment `json:"fetchDeployments,omitempty"`
		GetAuthConfig    *AuthConfig  `json:"authConfig,omitempty"`
		GetWorkspace     []Workspace  `json:"teams,omitempty"`
	} `json:"data"`
	Errors []Error `json:"errors,omitempty"`
}

type AuthUser struct {
	User  User  `json:"user"`
	Token Token `json:"token"`
}

// Decoded defines structure of a houston response Decoded object
type Decoded struct {
	ID  string `json:"id"`
	SU  bool   `json:"sU"`
	Iat int    `json:"iat"`
	Exp int    `json:"exp"`
}

// Deployment defines structure of a houston response Deployment object
type Deployment struct {
	Id          string `json:"uuid"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	ReleaseName string `json:"release_name"`
	Version     string `json:"version"`
}

// Email
type Email struct {
	Address  string `json:"address"`
	Verified bool   `json:"verified"`
	Primary  bool   `json:"primary"`
	// created at
	// updated at
}

// Error defines struct of a houston response Error object
type Error struct {
	Message string `json:"message"`
}

// AuthConfig holds data related to oAuth and basic authentication
type AuthConfig struct {
	LocalEnabled  bool   `json:"localEnabled"`
	GoogleEnabled bool   `json:"googleEnabled"`
	OauthUrl      string `json:"googleOAuthUrl"`
}

// Status defines structure of a houston response StatusResponse object
type Status struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Code    string `json:"code"`
	Id      string `json:"id"`
}

// Token defines structure of a houston response token object
type Token struct {
	Value   string       `json:"value"`
	Payload TokenPayload `json:"payload"`
}

type TokenPayload struct {
	Uuid string `json:"uuid"`
	Iat  int    `json:"iat"`
	Exp  int    `json:"exp"`
}

type User struct {
	Uuid     string  `json:"uuid"`
	Emails   []Email `json:"emails"`
	Username string  `json:"username"`
	Status   string  `json:"status"`
	// created at
	// updated at
	// profile
}

type Workspace struct {
	Uuid        string `json:"uuid"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
	Users       []User `json:"users"`
	// groups
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}
