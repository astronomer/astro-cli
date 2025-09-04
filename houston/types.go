package houston

import (
	"fmt"
	"time"

	semver "github.com/Masterminds/semver/v3"
)

// Response wraps all houston response structs used for json marashalling
type Response struct {
	Data   ResponseData `json:"data"`
	Errors []Error      `json:"errors,omitempty"`
}

type ResponseData struct {
	AddDeploymentUser              *RoleBinding                `json:"deploymentAddUserRole,omitempty"`
	DeleteDeploymentUser           *RoleBinding                `json:"deploymentRemoveUserRole,omitempty"`
	UpdateDeploymentUser           *RoleBinding                `json:"deploymentUpdateUserRole,omitempty"`
	DeploymentUserList             []DeploymentUser            `json:"deploymentUsers,omitempty"`
	AddWorkspaceUser               *Workspace                  `json:"workspaceAddUser,omitempty"`
	RemoveWorkspaceUser            *Workspace                  `json:"workspaceRemoveUser,omitempty"`
	CreateDeployment               *Deployment                 `json:"createDeployment,omitempty"`
	UpsertDeployment               *Deployment                 `json:"upsertDeployment,omitempty"`
	CreateToken                    *AuthUser                   `json:"createToken,omitempty"`
	CreateWorkspaceServiceAccount  *WorkspaceServiceAccount    `json:"createWorkspaceServiceAccount,omitempty"`
	CreateDeploymentServiceAccount *DeploymentServiceAccount   `json:"createDeploymentServiceAccount,omitempty"`
	CreateUser                     *AuthUser                   `json:"createUser,omitempty"`
	CreateWorkspace                *Workspace                  `json:"createWorkspace,omitempty"`
	DeleteDeployment               *Deployment                 `json:"deleteDeployment,omitempty"`
	DeleteWorkspaceServiceAccount  *ServiceAccount             `json:"deleteWorkspaceServiceAccount,omitempty"`
	DeleteDeploymentServiceAccount *ServiceAccount             `json:"deleteDeploymentServiceAccount,omitempty"`
	DeleteWorkspace                *Workspace                  `json:"deleteWorkspace,omitempty"`
	GetDeployment                  Deployment                  `json:"deployment,omitempty"`
	GetDeployments                 []Deployment                `json:"workspaceDeployments,omitempty"`
	PaginatedDeployments           []Deployment                `json:"paginatedDeployments,omitempty"`
	GetAuthConfig                  *AuthConfig                 `json:"authConfig,omitempty"`
	GetAppConfig                   *AppConfig                  `json:"appConfig,omitempty"`
	GetDeploymentServiceAccounts   []ServiceAccount            `json:"deploymentServiceAccounts,omitempty"`
	GetWorkspaceServiceAccounts    []ServiceAccount            `json:"workspaceServiceAccounts,omitempty"`
	GetUsers                       []User                      `json:"users,omitempty"`
	GetWorkspaces                  []Workspace                 `json:"workspaces,omitempty"`
	GetPaginatedWorkspaces         []Workspace                 `json:"paginatedWorkspaces,omitempty"`
	GetWorkspace                   *Workspace                  `json:"workspace,omitempty"`
	UpdateDeployment               *Deployment                 `json:"updateDeployment,omitempty"`
	UpdateDeploymentAirflow        *Deployment                 `json:"updateDeploymentAirflow,omitempty"`
	UpdateDeploymentRuntime        *Deployment                 `json:"updateDeploymentRuntime,omitempty"`
	CancelUpdateDeploymentRuntime  *Deployment                 `json:"cancelRuntimeUpdate,omitempty"`
	UpdateWorkspace                *Workspace                  `json:"updateWorkspace,omitempty"`
	DeploymentLog                  []DeploymentLog             `json:"logs,omitempty"`
	WorkspaceUpsertUserRole        string                      `json:"workspaceUpsertUserRole,omitempty"`
	WorkspaceGetUser               WorkspaceUserRoleBindings   `json:"workspaceUser,omitempty"`
	WorkspaceGetUsers              []WorkspaceUserRoleBindings `json:"workspaceUsers,omitempty"`
	WorkspacePaginatedGetUsers     []WorkspaceUserRoleBindings `json:"paginatedWorkspaceUsers,omitempty"`
	DeploymentConfig               DeploymentConfig            `json:"deploymentConfig,omitempty"`
	GetDeploymentNamespaces        []Namespace                 `json:"availableNamespaces,omitempty"`
	RuntimeReleases                RuntimeReleases             `json:"runtimeReleases,omitempty"`
	GetTeam                        *Team                       `json:"team,omitempty"`
	GetTeamUsers                   []User                      `json:"teamUsers,omitempty"`
	AddWorkspaceTeam               *Workspace                  `json:"workspaceAddTeam,omitempty"`
	RemoveWorkspaceTeam            *Workspace                  `json:"workspaceRemoveTeam,omitempty"`
	WorkspaceUpdateTeamRole        string                      `json:"workspaceUpdateTeamRole,omitempty"`
	WorkspaceGetTeams              []Team                      `json:"workspaceTeams,omitempty"`
	UpdateDeploymentImage          UpdateDeploymentImageResp   `json:"updateDeploymentImage,omitempty"`
	ListTeams                      ListTeamsResp               `json:"paginatedTeams,omitempty"`
	CreateTeamSystemRoleBinding    RoleBinding                 `json:"createTeamSystemRoleBinding"`
	DeleteTeamSystemRoleBinding    RoleBinding                 `json:"deleteTeamSystemRoleBinding"`
	AddDeploymentTeam              *RoleBinding                `json:"deploymentAddTeamRole,omitempty"`
	RemoveDeploymentTeam           *RoleBinding                `json:"deploymentRemoveTeamRole,omitempty"`
	UpdateDeploymentTeam           *RoleBinding                `json:"deploymentUpdateTeamRole,omitempty"`
	DeploymentGetTeams             []Team                      `json:"deploymentTeams,omitempty"`
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
	ID                    string              `json:"id"`
	Type                  string              `json:"type"`
	Label                 string              `json:"label"`
	ReleaseName           string              `json:"releaseName"`
	Version               string              `json:"version"`
	AirflowVersion        string              `json:"airflowVersion"`
	DesiredAirflowVersion string              `json:"desiredAirflowVersion"`
	RuntimeVersion        string              `json:"runtimeVersion"`
	RuntimeAirflowVersion string              `json:"runtimeAirflowVersion"`
	DesiredRuntimeVersion string              `json:"desiredRuntimeVersion"`
	DeploymentInfo        DeploymentInfo      `json:"deployInfo"`
	Workspace             Workspace           `json:"workspace"`
	Urls                  []DeploymentURL     `json:"urls"`
	CreatedAt             time.Time           `json:"createdAt"`
	UpdatedAt             time.Time           `json:"updatedAt"`
	DagDeployment         DagDeploymentConfig `json:"dagDeployment"`
	ClusterID             string              `json:"clusterId"`
}

type DagDeploymentConfig struct {
	Type string `json:"type"`
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

// DeploymentTeam defines a structure of RBAC deployment teams
type DeploymentTeam struct {
	ID           string        `json:"id"`
	RoleBindings []RoleBinding `json:"roleBindings"`
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
	Teams    []Team  `json:"teams"`
	// created at
	// updated at
	// profile
}

// Team contains all components of an Astronomer Team
type Team struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	SortID       int           `json:"sortId"`
	CreatedAt    string        `json:"createdAt"`
	UpdatedAt    string        `json:"updatedAt"`
	RoleBindings []RoleBinding `json:"roleBindings"`
}

type ListTeamsResp struct {
	Count int    `json:"count"`
	Teams []Team `json:"teams"`
}

type WorkspaceUserRoleBindings struct {
	ID           string        `json:"id"`
	Username     string        `json:"username"`
	FullName     string        `json:"fullName"`
	Emails       []Email       `json:"emails"`
	RoleBindings []RoleBinding `json:"roleBindings"`
}

type RoleBinding struct {
	Role           string                  `json:"role"`
	User           RoleBindingUser         `json:"user"`
	ServiceAccount WorkspaceServiceAccount `json:"serviceAccount"`
	Deployment     Deployment              `json:"deployment"`
	Team           Team                    `json:"team"`
	Workspace      Workspace               `json:"workspace"`
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

type UpdateDeploymentImageResp struct {
	ReleaseName    string `json:"releaseName"`
	AirflowVersion string `json:"airflowVersion"`
	RuntimeVersion string `json:"runtimeVersion"`
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
	BYORegistryDomain      string       `json:"byoUpdateRegistryHost"`
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
	BYORegistryEnabled     bool `json:"byoUpdateRegistryEnabled"`
	AstroRuntimeEnabled    bool `json:"astroRuntimeEnabled"`
	DagOnlyDeployment      bool `json:"dagOnlyDeployment"`
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

// RuntimeRelease contains info releated to a runtime release
type RuntimeRelease struct {
	Version           string `json:"version"`
	AirflowVersion    string `json:"airflowVersion"`
	AirflowDBMigraion bool   `json:"airflowDatabaseMigrations"`
}

type RuntimeReleases []RuntimeRelease

func (r RuntimeReleases) IsValidVersion(version string) bool {
	for idx := range r {
		if r[idx].Version == version {
			return true
		}
	}
	return false
}

func (r RuntimeReleases) GreaterVersions(version string) []string {
	greaterVersions := []string{}
	currentVersion, err := coerce(version)
	if err != nil {
		return greaterVersions
	}
	for idx := range r {
		runtimeVersion, err := coerce(r[idx].Version)
		if err != nil {
			continue
		}
		if runtimeVersion.Compare(currentVersion) >= 0 {
			greaterVersions = append(greaterVersions, r[idx].Version)
		}
	}
	return greaterVersions
}
