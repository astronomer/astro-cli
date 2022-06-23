package astro

import "time"

// Response wraps all astro response structs used for json marashalling
type Response struct {
	Data   ResponseData `json:"data"`
	Errors []Error      `json:"errors,omitempty"`
}

type ResponseData struct {
	CreateImage               *Image                       `json:"imageCreate,omitempty"`
	DeployImage               *Image                       `json:"imageDeploy,omitempty"`
	GetDeployment             Deployment                   `json:"deployment,omitempty"`
	GetDeployments            []Deployment                 `json:"deployments,omitempty"`
	GetWorkspaces             []Workspace                  `json:"workspaces,omitempty"`
	GetClusters               []Cluster                    `json:"clusters,omitempty"`
	SelfQuery                 *Self                        `json:"self,omitempty"`
	RuntimeReleases           []RuntimeRelease             `json:"runtimeReleases,omitempty"`
	DeploymentCreate          Deployment                   `json:"DeploymentCreate,omitempty"`
	GetDeploymentConfig       DeploymentConfig             `json:"deploymentConfigOptions,omitempty"`
	GetDeploymentHistory      DeploymentHistory            `json:"deploymentHistory,omitempty"`
	DeploymentDelete          Deployment                   `json:"deploymentDelete,omitempty"`
	DeploymentUpdate          Deployment                   `json:"deploymentUpdate,omitempty"`
	DeploymentVariablesUpdate []EnvironmentVariablesObject `json:"deploymentVariablesUpdate,omitempty"`
	InitiateDagDeployment	  InitiateDagDeployment		   `json:"initiateDagDeployment,omitempty"`
}

type Self struct {
	User User `json:"user"`
}

type AuthProvider struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	URL         string `json:"url"`
}

// AuthConfig holds data related to oAuth and basic authentication
type AuthConfig struct {
	ClientID  string `json:"clientId"`
	Audience  string `json:"audience"`
	DomainURL string `json:"domainUrl"`
}

type AuthUser struct {
	User  User  `json:"user"`
	Token Token `json:"token"`
}

// Deployment defines structure of a astrohub response Deployment object
type Deployment struct {
	ID              string         `json:"id"`
	Label           string         `json:"label"`
	Description     string         `json:"description"`
	WebserverStatus string         `json:"webserverStatus"`
	Status          string         `json:"status"`
	ReleaseName     string         `json:"releaseName"`
	Version         string         `json:"version"`
	Orchestrator    Orchestrator   `json:"orchestrator"`
	Workspace       Workspace      `json:"workspace"`
	RuntimeRelease  RuntimeRelease `json:"runtimeRelease"`
	DeploymentSpec  DeploymentSpec `json:"deploymentSpec"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       string         `json:"updatedAt"`
}

// Cluster contains all components of an Astronomer Cluster
type Cluster struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	CloudProvider string `json:"cloudProvider"`
}

type Orchestrator struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	CloudProvider string `json:"cloudProvider"`
}

type RuntimeRelease struct {
	Version                  string `json:"version"`
	AirflowVersion           string `json:"airflowVersion"`
	Channel                  string `json:"channel"`
	ReleaseDate              string `json:"releaseDate"`
	AirflowDatabaseMigration bool   `json:"airflowDatabaseMigration"`
}

type DeploymentSpec struct {
	Image                       Image                        `json:"image,omitempty"`
	Webserver                   Webserver                    `json:"webserver,omitempty"`
	Executor                    string                       `json:"executor"`
	Workers                     Workers                      `json:"workers"`
	Scheduler                   Scheduler                    `json:"scheduler"`
	EnvironmentVariablesObjects []EnvironmentVariablesObject `json:"environmentVariablesObjects"`
}

type InitiateDagDeployment struct {
	DagUrl string `json:"dagUrl"`
}

type InitiateDagDeploymentInput struct {
	OrganizationID string `json:"organizationId"`
	DeploymentID   string `json:"deploymentId"`
}

type EnvironmentVariablesObject struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	IsSecret  bool   `json:"isSecret"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// DeploymentURL defines structure of a astrohub response DeploymentURL object
type DeploymentURL struct {
	Type string `json:"type"`
	URL  string `json:"url"`
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
	ID      string `json:"id"`
}

// DeploymentUser defines a structure of RBAC deployment users
type DeploymentUser struct {
	ID           string        `json:"id"`
	FullName     string        `json:"fullName"`
	Username     string        `json:"username"`
	RoleBindings []RoleBinding `json:"roleBindings"`
}

// Token contains a astrohub auth token as well as it's payload of components
type Token struct {
	Value string `json:"value"`
}

// User contains all components of an Astronomer user
type User struct {
	ID           string        `json:"id"`
	RoleBindings []RoleBinding `json:"roleBindings"`
}

type RoleBinding struct {
	Role string `json:"role"`
	User struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"user"`
	Deployment Deployment `json:"deployment"`
}

// Workspace contains all components of an Astronomer Workspace
type Workspace struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	Description    string `json:"description"`
	Users          []User `json:"users"`
	OrganizationID string `json:"organizationId"`
	// groups
	CreatedAt    string        `json:"createdAt"`
	UpdatedAt    string        `json:"updatedAt"`
	RoleBindings []RoleBinding `json:"roleBindings"`
}

type Image struct {
	ID           string   `json:"id"`
	DeploymentID string   `json:"deploymentId"`
	Digest       string   `json:"digest"`
	Env          []string `json:"env"`
	Labels       []string `json:"labels"`
	Name         string   `json:"name"`
	Tag          string   `json:"tag"`
	Repository   string   `json:"repository"`
	CreatedAt    string   `json:"createdAt"`
}

type Webserver struct {
	URL string `json:"url"`
}

type Workers struct {
	AU                            int `json:"au"`
	TerminationGracePeriodSeconds int `json:"terminationGracePeriodSeconds"`
}

type Scheduler struct {
	AU       int `json:"au"`
	Replicas int `json:"replicas"`
}

type DeploymentConfig struct {
	AstronomerUnit  AstronomerUnit   `json:"astroUnit"`
	RuntimeReleases []RuntimeRelease `json:"runtimeReleases"`
}

type AstronomerUnit struct {
	CPU    int `json:"cpu"`
	Memory int `json:"memory"`
}

type DeploymentHistory struct {
	DeploymentID  string         `json:"deploymentId"`
	ReleaseName   string         `json:"releaseName"`
	SchedulerLogs []SchedulerLog `json:"schedulerLogs"`
}

type SchedulerLog struct {
	Timestamp string `json:"timestamp"`
	Raw       string `json:"raw"`
	Level     string `json:"level"`
}

type DeploymentsInput struct {
	WorkspaceID  string `json:"workspaceId"`
	DeploymentID string `json:"deploymentId"`
}

type ImageCreateInput struct {
	Tag          string `json:"tag"`
	DeploymentID string `json:"deploymentId"`
}

type ImageDeployInput struct {
	ID         string `json:"id"`
	Tag        string `json:"tag"`
	Repository string `json:"repository"`
}

type DeploymentCreateInput struct {
	WorkspaceID           string               `json:"workspaceId"`
	OrchestratorID        string               `json:"orchestratorId"`
	Label                 string               `json:"label"`
	Description           string               `json:"description"`
	AirflowTag            string               `json:"airflowTag"`
	RuntimeReleaseVersion string               `json:"runtimeReleaseVersion"`
	DeploymentSpec        DeploymentCreateSpec `json:"deploymentSpec"`
}

type DeploymentCreateSpec struct {
	Executor  string    `json:"executor"`
	Workers   Workers   `json:"workers"`
	Scheduler Scheduler `json:"scheduler"`
}

type DeploymentUpdateInput struct {
	ID             string               `json:"id"`
	OrchestratorID string               `json:"orchestratorId"`
	Label          string               `json:"label"`
	Description    string               `json:"description"`
	DeploymentSpec DeploymentCreateSpec `json:"deploymentSpec"`
}

type DeploymentDeleteInput struct {
	ID string `json:"id"`
}

type EnvironmentVariablesInput struct {
	DeploymentID         string                `json:"deploymentId"`
	EnvironmentVariables []EnvironmentVariable `json:"environmentVariables"`
}

type EnvironmentVariable struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	IsSecret bool   `json:"isSecret"`
}
