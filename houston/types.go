package houston

// Response wraps all houston response structs used for json marashalling
type Response struct {
	Data struct {
		AddWorkspaceUser        *Workspace       `json:"workspaceAddUser,omitempty"`
		RemoveWorkspaceUser     *Workspace       `json:"workspaceRemoveUser,omitempty"`
		CreateDeployment        *Deployment      `json:"createDeployment,omitempty"`
		CreateToken             *AuthUser        `json:"createToken,omitempty"`
		CreateServiceAccount    *ServiceAccount  `json:"createServiceAccount,omitempty"`
		CreateUser              *AuthUser        `json:"createUser,omitempty"`
		CreateWorkspace         *Workspace       `json:"createWorkspace,omitempty"`
		DeleteDeployment        *Deployment      `json:"deleteDeployment,omitempty"`
		DeleteServiceAccount    *ServiceAccount  `json:"deleteServiceAccount,omitempty"`
		DeleteWorkspace         *Workspace       `json:"deleteWorkspace,omitempty"`
		GetDeployments          []Deployment     `json:"deployments,omitempty"`
		GetAuthConfig           *AuthConfig      `json:"authConfig,omitempty"`
		GetServiceAccounts      []ServiceAccount `json:"serviceAccounts,omitempty"`
		GetUsers                []User           `json:"users,omitempty"`
		GetWorkspaces           []Workspace      `json:"workspaces,omitempty"`
		UpdateDeployment        *Deployment      `json:"updateDeployment,omitempty"`
		UpdateWorkspace         *Workspace       `json:"updateWorkspace,omitempty"`
		DeploymentLog           []DeploymentLog  `json:"logs,omitempty"`
		WorkspaceUpdateUserRole string           `json:"workspaceUpdateUserRole,omitempty"`
	} `json:"data"`
	Errors []Error `json:"errors,omitempty"`
}

// AuthConfig holds data related to oAuth and basic authentication
type AuthConfig struct {
	LocalEnabled  bool `json:"localEnabled"`
	GoogleEnabled bool `json:"googleEnabled"`
	GithubEnabled bool `json:"githubEnabled"`
	Auth0Enabled  bool `json:"auth0Enabled"`
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
	Id             string         `json:"id"`
	Type           string         `json:"type"`
	Label          string         `json:"label"`
	ReleaseName    string         `json:"releaseName"`
	Version        string         `json:"version"`
	DeploymentInfo DeploymentInfo `json:"deployInfo"`
	Workspace      Workspace      `json:"workspace"`
	CreatedAt      string         `json:"createdAt"`
	UpdatedAt      string         `json:"updatedAt"`
}

// DeploymentInfo contains registry related information for a deployment
type DeploymentInfo struct {
	Latest string `json:"latest"`
	Next   string `json:"next"`
}

// Email contains various pieces of a users email
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
	Name    string `json:"name"`
}

// Status defines structure of a houston response StatusResponse object
type Status struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Code    string `json:"code"`
	Id      string `json:"id"`
}

// ServiceACcount defines a structure of a ServiceAccountResponse object
type ServiceAccount struct {
	Id         string `json:"id"`
	ApiKey     string `json:"apiKey"`
	Label      string `json:"label"`
	Category   string `json:"category"`
	EntityType string `json:"entityType"`
	EntityId   string `json:"entityId"`
	LastUsedAt string `json:"lastUsedAt"`
	Active     bool   `json:"active"`
}

// Token contains a houston auth token as well as it's payload of components
type Token struct {
	Value   string       `json:"value"`
	Payload TokenPayload `json:"payload"`
}

// TokenPayload contains components of a houston auth token
type TokenPayload struct {
	Id  string `json:"id"`
	Iat int    `json:"iat"`
	Exp int    `json:"exp"`
}

// User contains all components of an Astronomer user
type User struct {
	Id       string  `json:"id"`
	Emails   []Email `json:"emails"`
	Username string  `json:"username"`
	Status   string  `json:"status"`
	// created at
	// updated at
	// profile
}

type RoleBinding struct {
	Role string `json:"role"`
	User struct {
		Id       string `json:"id"`
		Username string `json:"username"`
	} `json:"user"`
}

// Workspace contains all components of an Astronomer Workspace
type Workspace struct {
	Id          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Users       []User `json:"users"`
	// groups
	CreatedAt    string        `json:"createdAt"`
	UpdatedAt    string        `json:"updatedAt"`
	RoleBindings []RoleBinding `json:"roleBindings"`
}

// DeploymentLog contains all log related to deployment components
type DeploymentLog struct {
	Id        string `json:"id"`
	Component string `json:"component"`
	CreatedAt string `json:"createdAt"`
	Log       string `json:"log"`
}
