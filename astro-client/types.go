package astro

import "time"

// Response wraps all astro response structs used for json marashalling
type Response struct {
	Data   ResponseData `json:"data"`
	Errors []Error      `json:"errors,omitempty"`
}

type ResponseData struct {
	CreateImage               *Image                       `json:"createImage,omitempty"`
	DeployImage               *Image                       `json:"deployImage,omitempty"`
	GetDeployment             Deployment                   `json:"deployment,omitempty"`
	GetDeployments            []Deployment                 `json:"deployments,omitempty"`
	GetWorkspaces             []Workspace                  `json:"workspaces,omitempty"`
	GetWorkspace              Workspace                    `json:"workspace,omitempty"`
	RuntimeReleases           []RuntimeRelease             `json:"runtimeReleases,omitempty"`
	CreateDeployment          Deployment                   `json:"CreateDeployment,omitempty"`
	GetDeploymentConfig       DeploymentConfig             `json:"deploymentConfigOptions,omitempty"`
	GetDeploymentHistory      DeploymentHistory            `json:"deploymentHistory,omitempty"`
	DeleteDeployment          Deployment                   `json:"DeleteDeployment,omitempty"`
	UpdateDeployment          Deployment                   `json:"UpdateDeployment,omitempty"`
	UpdateDeploymentVariables []EnvironmentVariablesObject `json:"UpdateDeploymentVariables,omitempty"`
	InitiateDagDeployment     InitiateDagDeployment        `json:"initiateDagDeployment,omitempty"`
	ReportDagDeploymentStatus DagDeploymentStatus          `json:"reportDagDeploymentStatus,omitempty"`
	GetWorkerQueueOptions     WorkerQueueDefaultOptions    `json:"workerQueueOptions,omitempty"`
	DeploymentAlerts          DeploymentAlerts             `json:"alertEmails,omitempty"`
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
	ID                    string         `json:"id"`
	Label                 string         `json:"label"`
	Description           string         `json:"description"`
	WebserverStatus       string         `json:"webserverStatus"`
	Status                string         `json:"status"`
	ReleaseName           string         `json:"releaseName"`
	Version               string         `json:"version"`
	Type                  string         `json:"type"`
	DagDeployEnabled      bool           `json:"dagDeployEnabled"`
	APIKeyOnlyDeployments bool           `json:"apiKeyOnlyDeployments"`
	AlertEmails           []string       `json:"alertEmails"`
	Cluster               Cluster        `json:"cluster"`
	Workspace             Workspace      `json:"workspace"`
	RuntimeRelease        RuntimeRelease `json:"runtimeRelease"`
	DeploymentSpec        DeploymentSpec `json:"deploymentSpec"`
	SchedulerSize         string         `json:"schedulerSize"`
	IsHighAvailability    bool           `json:"isHighAvailability"`
	WorkerQueues          []WorkerQueue  `json:"workerQueues"`
	CreatedAt             time.Time      `json:"createdAt"`
	UpdatedAt             time.Time      `json:"updatedAt"`
}

// Cluster contains all components of an Astronomer Cluster
type Cluster struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	CloudProvider string     `json:"cloudProvider"`
	Region        string     `json:"region"`
	NodePools     []NodePool `json:"nodePools"`
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
	Scheduler                   Scheduler                    `json:"scheduler"`
	EnvironmentVariablesObjects []EnvironmentVariablesObject `json:"environmentVariablesObjects"`
}

type InitiateDagDeployment struct {
	ID     string `json:"id"`
	DagURL string `json:"dagUrl"`
}

type InitiateDagDeploymentInput struct {
	RuntimeID string `json:"runtimeId"`
}

type DagDeploymentStatus struct {
	ID            string `json:"id"`
	RuntimeID     string `json:"runtimeId"`
	Action        string `json:"action"`
	VersionID     string `json:"versionId"`
	Status        string `json:"status"`
	Message       string `json:"message"`
	CreatedAt     string `json:"createdAt"`
	InitiatorID   string `json:"initiatorId"`
	InitiatorType string `json:"initiatorType"`
}

type ReportDagDeploymentStatusInput struct {
	InitiatedDagDeploymentID string `json:"initiatedDagDeploymentId"`
	RuntimeID                string `json:"runtimeId"`
	Action                   string `json:"action"`
	VersionID                string `json:"versionId"`
	Status                   string `json:"status"`
	Message                  string `json:"message"`
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

type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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

type Scheduler struct {
	AU       int `json:"au"`
	Replicas int `json:"replicas"`
}

type AuConfig struct {
	Default int `json:"default"`
	Limit   int `json:"limit"`
	Request int `json:"request"`
}

type ReplicasConfig struct {
	Default int `json:"default"`
	Limit   int `json:"limit"`
	Minimum int `json:"minimum"`
}

type WebserverConfig struct {
	AU AuConfig `json:"au"`
}

type SchedulerConfig struct {
	AU       AuConfig       `json:"au"`
	Replicas ReplicasConfig `json:"replicas"`
}

type CeleryExecutorConfig struct {
	Name string `json:"name"`
}

type KubernetesExecutorConfig struct {
	Name string `json:"name"`
}

type ExecutorConfig struct {
	CeleryExecutor     CeleryExecutorConfig     `json:"celeryExecutor"`
	KubernetesExecutor KubernetesExecutorConfig `json:"kubernetesExecutor"`
}

type Components struct {
	Scheduler SchedulerConfig `json:"scheduler"`
	Webserver WebserverConfig `json:"webserver"`
	Executor  ExecutorConfig  `json:"executor"`
}

type DeploymentConfig struct {
	AstronomerUnit       AstronomerUnit   `json:"astroUnit"`
	RuntimeReleases      []RuntimeRelease `json:"runtimeReleases"`
	AstroMachines        []Machine        `json:"astroMachines"`
	DefaultAstroMachine  Machine          `json:"defaultAstroMachine"`
	SchedulerSizes       []MachineUnit    `json:"schedulerSizes"`
	DefaultSchedulerSize MachineUnit      `json:"defaultSchedulerSize"`
	Components           Components       `json:"components"`
}

type Machine struct {
	Type            string `json:"type"`
	CPU             string `json:"cpu"`
	Memory          string `json:"memory"`
	StorageSize     string `json:"storageSize"`
	ConcurrentTasks int    `json:"concurrentTasks"`
	NodePoolType    string `json:"nodePoolType"`
}

type MachineUnit struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
	Size   string `json:"size"`
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

type CreateImageInput struct {
	Tag          string `json:"tag"`
	DeploymentID string `json:"deploymentId"`
}

type DeployImageInput struct {
	ImageID          string `json:"imageId"`
	DeploymentID     string `json:"deploymentId"`
	Tag              string `json:"tag"`
	Repository       string `json:"repository"`
	DagDeployEnabled bool   `json:"dagDeployEnabled"`
}

type CreateDeploymentInput struct {
	WorkspaceID           string               `json:"workspaceId"`
	ClusterID             string               `json:"clusterId"`
	Label                 string               `json:"label"`
	Description           string               `json:"description"`
	RuntimeReleaseVersion string               `json:"runtimeReleaseVersion"`
	DagDeployEnabled      bool                 `json:"dagDeployEnabled"`
	DeploymentSpec        DeploymentCreateSpec `json:"deploymentSpec"`
	WorkerQueues          []WorkerQueue        `json:"workerQueues"`
	IsHighAvailability    bool                 `json:"isHighAvailability"`
	SchedulerSize         string               `json:"schedulerSize"`
	APIKeyOnlyDeployments bool                 `json:"apiKeyOnlyDeployments"`
}

type DeploymentCreateSpec struct {
	Executor  string    `json:"executor"`
	Scheduler Scheduler `json:"scheduler"`
}

type UpdateDeploymentInput struct {
	ID                    string               `json:"id"`
	ClusterID             string               `json:"clusterId"`
	Label                 string               `json:"label"`
	Description           string               `json:"description"`
	DagDeployEnabled      bool                 `json:"dagDeployEnabled"`
	APIKeyOnlyDeployments bool                 `json:"apiKeyOnlyDeployments"`
	DeploymentSpec        DeploymentCreateSpec `json:"deploymentSpec"`
	IsHighAvailability    bool                 `json:"isHighAvailability"`
	SchedulerSize         string               `json:"schedulerSize"`
	WorkerQueues          []WorkerQueue        `json:"workerQueues"`
}

type DeleteDeploymentInput struct {
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

// Input for creating a user invite
type CreateUserInviteInput struct {
	InviteeEmail   string `json:"inviteeEmail"`
	Role           string `json:"role"`
	OrganizationID string `json:"organizationId"`
}

// Output returned when a user invite is created
type UserInvite struct {
	UserID         string `json:"userId"`
	OrganizationID string `json:"organizationId"`
	OauthInviteID  string `json:"oauthInviteId"`
	ExpiresAt      string `json:"expiresAt"`
}

type WorkerQueue struct {
	ID                string `json:"id,omitempty"` // Empty when creating new WorkerQueues
	Name              string `json:"name"`
	AstroMachine      string `json:"astroMachine"`
	IsDefault         bool   `json:"isDefault"`
	MaxWorkerCount    int    `json:"maxWorkerCount,omitempty"`
	MinWorkerCount    int    `json:"minWorkerCount"`
	WorkerConcurrency int    `json:"workerConcurrency,omitempty"`
	NodePoolID        string `json:"nodePoolId"`
	PodCPU            string `json:"podCpu,omitempty"`
	PodRAM            string `json:"podRam,omitempty"`
}

type WorkerQueueDefaultOptions struct {
	MinWorkerCount    WorkerQueueOption `json:"minWorkerCount"`
	MaxWorkerCount    WorkerQueueOption `json:"maxWorkerCount"`
	WorkerConcurrency WorkerQueueOption `json:"workerConcurrency"`
}

type WorkerQueueOption struct {
	Floor   int `json:"floor"`
	Ceiling int `json:"ceiling"`
	Default int `json:"default"`
}

type NodePool struct {
	ID               string    `json:"id"`
	IsDefault        bool      `json:"isDefault"`
	NodeInstanceType string    `json:"nodeInstanceType"`
	CreatedAt        time.Time `json:"createdAt"`
}

type UpdateDeploymentAlertsInput struct {
	DeploymentID string   `json:"deploymentId"`
	AlertEmails  []string `json:"alertEmails"`
}

type DeploymentAlerts struct {
	AlertEmails []string `json:"alertEmails"`
}
