package houston

import (
	"fmt"

	semver "github.com/Masterminds/semver/v3"
)

// Response wraps all houston response structs used for json marashalling
type Response struct {
	Data   ResponseData `json:"data"`
	Errors []Error      `json:"errors,omitempty"`
}

type ResponseData struct {
	AddDeploymentUser              *RoleBinding              `json:"deploymentAddUserRole,omitempty"`
	DeleteDeploymentUser           *RoleBinding              `json:"deploymentRemoveUserRole,omitempty"`
	UpdateDeploymentUser           *RoleBinding              `json:"deploymentUpdateUserRole,omitempty"`
	DeploymentUserList             []DeploymentUser          `json:"deploymentUsers,omitempty"`
	AddWorkspaceUser               *Workspace                `json:"workspaceAddUser,omitempty"`
	RemoveWorkspaceUser            *Workspace                `json:"workspaceRemoveUser,omitempty"`
	CreateDeployment               *Deployment               `json:"createDeployment,omitempty"`
	CreateToken                    *AuthUser                 `json:"createToken,omitempty"`
	CreateWorkspaceServiceAccount  *WorkspaceServiceAccount  `json:"createWorkspaceServiceAccount,omitempty"`
	CreateDeploymentServiceAccount *DeploymentServiceAccount `json:"createDeploymentServiceAccount,omitempty"`
	CreateUser                     *AuthUser                 `json:"createUser,omitempty"`
	CreateWorkspace                *Workspace                `json:"createWorkspace,omitempty"`
	DeleteDeployment               *Deployment               `json:"deleteDeployment,omitempty"`
	DeleteWorkspaceServiceAccount  *ServiceAccount           `json:"deleteWorkspaceServiceAccount,omitempty"`
	DeleteDeploymentServiceAccount *ServiceAccount           `json:"deleteDeploymentServiceAccount,omitempty"`
	DeleteWorkspace                *Workspace                `json:"deleteWorkspace,omitempty"`
	GetDeployment                  Deployment                `json:"deployment,omitempty"`
	GetDeployments                 []Deployment              `json:"workspaceDeployments,omitempty"`
	GetAuthConfig                  *AuthConfig               `json:"authConfig,omitempty"`
	GetAppConfig                   *AppConfig                `json:"appConfig,omitempty"`
	GetDeploymentServiceAccounts   []ServiceAccount          `json:"deploymentServiceAccounts,omitempty"`
	GetWorkspaceServiceAccounts    []ServiceAccount          `json:"workspaceServiceAccounts,omitempty"`
	GetUsers                       []User                    `json:"users,omitempty"`
	GetWorkspaces                  []Workspace               `json:"workspaces,omitempty"`
	GetWorkspace                   *Workspace                `json:"workspace,omitempty"`
	UpdateDeployment               *Deployment               `json:"updateDeployment,omitempty"`
	UpdateDeploymentAirflow        *Deployment               `json:"updateDeploymentAirflow,omitempty"`
	UpdateWorkspace                *Workspace                `json:"updateWorkspace,omitempty"`
	DeploymentLog                  []DeploymentLog           `json:"logs,omitempty"`
	WorkspaceUpdateUserRole        string                    `json:"workspaceUpdateUserRole,omitempty"`
	WorkspaceGetUser               WorkspaceUserRoleBindings `json:"workspaceUser,omitempty"`
	DeploymentConfig               DeploymentConfig          `json:"deploymentConfig,omitempty"`
	GetDeploymentNamespaces        []Namespace               `json:"availableNamespaces,omitempty"`
	GetTeam                        *Team                     `json:"team,omitempty"`
	GetTeamUsers                   []User                    `json:"teamUsers,omitempty"`
}

type Namespace struct {
	Name string `json:"name"`
}

type AuthProvider struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	URL         string `json:"url"`
}

// AuthConfig holds data related to oAuth and basic authentication
type AuthConfig struct {
	LocalEnabled  bool           `json:"localEnabled"`
	PublicSignup  bool           `json:"publicSignup"`
	InitialSignup bool           `json:"initialSignup"`
	AuthProviders []AuthProvider `json:"providers"`
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
	ID                    string          `json:"id"`
	Type                  string          `json:"type"`
	Label                 string          `json:"label"`
	ReleaseName           string          `json:"releaseName"`
	Version               string          `json:"version"`
	AirflowVersion        string          `json:"airflowVersion"`
	DesiredAirflowVersion string          `json:"desiredAirflowVersion"`
	DeploymentInfo        DeploymentInfo  `json:"deployInfo"`
	Workspace             Workspace       `json:"workspace"`
	Urls                  []DeploymentURL `json:"urls"`
	CreatedAt             string          `json:"createdAt"`
	UpdatedAt             string          `json:"updatedAt"`
}

// DeploymentURL defines structure of a houston response DeploymentURL object
type DeploymentURL struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// DeploymentInfo contains registry related information for a deployment
type DeploymentInfo struct {
	NextCli string `json:"NextCli"`
	Current string `json:"current"`
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
	ID      string `json:"id"`
}

// ServiceAccount defines a structure of a ServiceAccountResponse object
type ServiceAccount struct {
	ID         string `json:"id"`
	APIKey     string `json:"apiKey"`
	Label      string `json:"label"`
	Category   string `json:"category"`
	LastUsedAt string `json:"lastUsedAt"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
	Active     bool   `json:"active"`
}

// WorkspaceServiceAccount defines a structure of a WorkspaceServiceAccountResponse object
type WorkspaceServiceAccount struct {
	ID            string `json:"id"`
	APIKey        string `json:"apiKey"`
	Label         string `json:"label"`
	Category      string `json:"category"`
	EntityType    string `json:"entityType"`
	WorkspaceUUID string `json:"workspaceUuid"`
	LastUsedAt    string `json:"lastUsedAt"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
	Active        bool   `json:"active"`
}

// DeploymentUser defines a structure of RBAC deployment users
type DeploymentUser struct {
	ID           string        `json:"id"`
	Emails       []Email       `json:"emails"`
	FullName     string        `json:"fullName"`
	Username     string        `json:"username"`
	RoleBindings []RoleBinding `json:"roleBindings"`
}

// DeploymentServiceAccount defines a structure of a DeploymentServiceAccountResponse object
type DeploymentServiceAccount struct {
	ID             string `json:"id"`
	APIKey         string `json:"apiKey"`
	Label          string `json:"label"`
	Category       string `json:"category"`
	EntityType     string `json:"entityType"`
	DeploymentUUID string `json:"deploymentUuid"`
	LastUsedAt     string `json:"lastUsedAt"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
	Active         bool   `json:"active"`
}

// Token contains a houston auth token as well as it's payload of components
type Token struct {
	Value   string       `json:"value"`
	Payload TokenPayload `json:"payload"`
}

// TokenPayload contains components of a houston auth token
type TokenPayload struct {
	ID  string `json:"id"`
	Iat int    `json:"iat"`
	Exp int    `json:"exp"`
}

// User contains all components of an Astronomer user
type User struct {
	ID       string  `json:"id"`
	Emails   []Email `json:"emails"`
	Username string  `json:"username"`
	Status   string  `json:"status"`
	// created at
	// updated at
	// profile
}

type RoleBindingWorkspace struct {
	Role      string `json:"role"`
	Workspace struct {
		ID string `json:"id"`
	} `json:"workspace"`
}

type WorkspaceUserRoleBindings struct {
	RoleBindings []RoleBindingWorkspace `json:"roleBindings"`
}

type RoleBinding struct {
	Role           string                  `json:"role"`
	User           RoleBindingUser         `json:"user"`
	ServiceAccount WorkspaceServiceAccount `json:"serviceAccount"`
	Deployment     Deployment              `json:"deployment"`
}

type RoleBindingUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

// Workspace contains all components of an Astronomer Workspace
type Workspace struct {
	ID          string `json:"id"`
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
	ID        string `json:"id"`
	Component string `json:"component"`
	CreatedAt string `json:"createdAt"`
	Log       string `json:"log"`
}

// AirflowImage contains all airflow image attributes
type AirflowImage struct {
	Version string `json:"version"`
	Tag     string `json:"tag"`
}

// DeploymentConfig contains current airflow image tag
type DeploymentConfig struct {
	AirflowImages          []AirflowImage `json:"airflowImages"`
	DefaultAirflowImageTag string         `json:"defaultAirflowImageTag"`
	AirflowVersions        []string       `json:"airflowVersions"`
}

func (config *DeploymentConfig) GetValidTags(tag string) (tags []string) {
	tagVersion, err := coerce(tag)
	// if tag doesn't follow the semver standard return empty array
	if err != nil {
		fmt.Println(err)
		return
	}

	for _, image := range config.AirflowImages {
		imageTagVersion, err := coerce(image.Version)
		if err != nil {
			continue
		}
		// i = 1 means version greater than
		if i := imageTagVersion.Compare(tagVersion); i >= 0 {
			tags = append(tags, image.Tag)
		}
	}
	return
}

func (config *DeploymentConfig) IsValidTag(tag string) bool {
	for _, validTag := range config.GetValidTags(tag) {
		if tag == validTag {
			return true
		}
	}
	return false
}

// AppConfig contains current houston config
type AppConfig struct {
	Version                string       `json:"version"`
	BaseDomain             string       `json:"baseDomain"`
	SMTPConfigured         bool         `json:"smtpConfigured"`
	ManualReleaseNames     bool         `json:"manualReleaseNames"`
	ConfigureDagDeployment bool         `json:"configureDagDeployment"`
	NfsMountDagDeployment  bool         `json:"nfsMountDagDeployment"`
	HardDeleteDeployment   bool         `json:"hardDeleteDeployment"`
	ManualNamespaceNames   bool         `json:"manualNamespaceNames"`
	TriggererEnabled       bool         `json:"triggererEnabled"`
	Flags                  FeatureFlags `json:"featureFlags,omitempty"`
}

type FeatureFlags struct {
	NfsMountDagDeployment  bool `json:"nfsMountDagDeployment"`
	HardDeleteDeployment   bool `json:"hardDeleteDeployment"`
	ManualNamespaceNames   bool `json:"manualNamespaceNames"`
	TriggererEnabled       bool `json:"triggererEnabled"`
	GitSyncEnabled         bool `json:"gitSyncDagDeployment"`
	NamespaceFreeFormEntry bool `json:"namespaceFreeFormEntry"`
}

// Team contains all components of an Astronomer Team
type Team struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	SortID       int           `json:"sortid"`
	CreatedAt    string        `json:"createdAt"`
	UpdatedAt    string        `json:"updatedAt"`
	RoleBindings []RoleBinding `json:"roleBindings"`
}

// coerce a string into SemVer if possible
func coerce(version string) (*semver.Version, error) {
	v, err := semver.NewVersion(version)
	if err != nil {
		return nil, err
	}
	coerceVer, err := semver.NewVersion(fmt.Sprintf("%d.%d.%d", v.Major(), v.Minor(), v.Patch()))
	if err != nil {
		return nil, err
	}
	return coerceVer, nil
}
