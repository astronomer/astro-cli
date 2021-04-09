package astrohub

import (
	"fmt"

	"github.com/Masterminds/semver"
)

// Response wraps all astrohub response structs used for json marashalling
type Response struct {
	Data struct {
		AddDeploymentUser              *RoleBinding              `json:"deploymentAddUserRole,omitempty"`
		DeleteDeploymentUser           *RoleBinding              `json:"deploymentRemoveUserRole,omitempty"`
		UpdateDeploymentUser           *RoleBinding              `json:"deploymentUpdateUserRole,omitempty"`
		DeploymentUserList             []DeploymentUser          `json:"deploymentUsers,omitempty"`
		AddWorkspaceUser               *Workspace                `json:"workspaceAddUser,omitempty"`
		RemoveWorkspaceUser            *Workspace                `json:"workspaceRemoveUser,omitempty"`
		CreateDeployment               *Deployment               `json:"createDeployment,omitempty"`
		CreateToken                    *AuthUser                 `json:"createToken,omitempty"`
		CreateUser                     *AuthUser                 `json:"createUser,omitempty"`
		CreateWorkspace                *Workspace                `json:"createWorkspace,omitempty"`
		DeleteDeployment               *Deployment               `json:"deleteDeployment,omitempty"`
		DeleteWorkspace                *Workspace                `json:"deleteWorkspace,omitempty"`
		GetDeployment                  Deployment                `json:"deployment,omitempty"`
		GetDeployments                 []Deployment              `json:"workspaceDeployments,omitempty"`
		GetAuthConfig                  *AuthConfig               `json:"authConfig,omitempty"`
		GetAppConfig                   *AppConfig                `json:"appConfig,omitempty"`
		GetUsers                       []User                    `json:"users,omitempty"`
		GetWorkspaces                  []Workspace               `json:"workspaces,omitempty"`
		UpdateDeployment               *Deployment               `json:"updateDeployment,omitempty"`
		UpdateDeploymentAirflow        *Deployment               `json:"updateDeploymentAirflow,omitempty"`
		UpdateWorkspace                *Workspace                `json:"updateWorkspace,omitempty"`
		WorkspaceUpdateUserRole        string                    `json:"workspaceUpdateUserRole,omitempty"`
		DeploymentConfig               DeploymentConfig          `json:"deploymentConfig,omitempty"`
	} `json:"data"`
	Errors []Error `json:"errors,omitempty"`
}

type AuthProvider struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Url         string `json:"url"`
}

// AuthConfig holds data related to oAuth and basic authentication
type AuthConfig struct {
	ClientId  string           `json:"clientId"`
	Audience  string           `json:"audience"`
	DomainUrl string           `json:"domainUrl"`
}

type AuthUser struct {
	User  User  `json:"user"`
	Token Token `json:"token"`
}

// Decoded defines structure of a astrohub response Decoded object
type Decoded struct {
	ID  string `json:"id"`
	SU  bool   `json:"sU"`
	Iat int    `json:"iat"`
	Exp int    `json:"exp"`
}

// Deployment defines structure of a astrohub response Deployment object
type Deployment struct {
	Id                    string          `json:"id"`
	Type                  string          `json:"type"`
	Label                 string          `json:"label"`
	ReleaseName           string          `json:"releaseName"`
	Version               string          `json:"version"`
	AirflowVersion        string          `json:"airflowVersion"`
	DesiredAirflowVersion string          `json:"desiredAirflowVersion"`
	DeploymentInfo        DeploymentInfo  `json:"deployInfo"`
	Workspace             Workspace       `json:"workspace"`
	Urls                  []DeploymentUrl `json:"urls"`
	CreatedAt             string          `json:"createdAt"`
	UpdatedAt             string          `json:"updatedAt"`
}

// DeploymentUrl defines structure of a astrohub response DeploymentUrl object
type DeploymentUrl struct {
	Type string `json:"type"`
	Url  string `json:"url"`
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

// Error defines struct of a astrohub response Error object
type Error struct {
	Message string `json:"message"`
	Name    string `json:"name"`
}

// Status defines structure of a astrohub response StatusResponse object
type Status struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Code    string `json:"code"`
	Id      string `json:"id"`
}

// DeploymentUser defines a structure of RBAC deployment users
type DeploymentUser struct {
	Id           string        `json:"id"`
	Emails       []Email       `json:"emails"`
	FullName     string        `json:"fullName"`
	Username     string        `json:"username"`
	RoleBindings []RoleBinding `json:"roleBindings"`
}

// Token contains a astrohub auth token as well as it's payload of components
type Token struct {
	Value   string       `json:"value"`
	Payload TokenPayload `json:"payload"`
}

// TokenPayload contains components of a astrohub auth token
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
	Deployment Deployment `json:"deployment"`
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
	for _, image := range config.AirflowImages {
		tagVersion := coerce(tag)
		imageTagVersion := coerce(image.Version)
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

// AppConfig contains current astrohub config
type AppConfig struct {
	Version            string `json:"version"`
	BaseDomain         string `json:"baseDomain"`
	SmtpConfigured     bool   `json:"smtpConfigured"`
	ManualReleaseNames bool   `json:"manualReleaseNames"`
}

// coerce a string into SemVer if possible
func coerce(version string) *semver.Version {
	v, err := semver.NewVersion(version)
	if err != nil {
		fmt.Println(err)
	}
	coerceVer, err := semver.NewVersion(fmt.Sprintf("%d.%d.%d", v.Major(), v.Minor(), v.Patch()))
	if err != nil {
		fmt.Println(err)
	}
	return coerceVer
}
