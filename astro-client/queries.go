package astro

// TODO: @adam2k Reorganize based on this issue - https://github.com/astronomer/issues/issues/1991
var (
	WorkspacesGetRequest = `
	query GetWorkspaces {
		workspaces {
			id
			label
			organizationId
		}
	}`

	WorkspaceDeploymentsGetRequest = `
	query WorkspaceDeployments(
		$deploymentsInput: DeploymentsInput
	) {
		deployments(input: $deploymentsInput) {
			id
			label
			releaseName
			orchestrator {
				id
			}
			createdAt
			runtimeRelease {
				version
				airflowVersion
			}
			deploymentSpec {
				image {
					tag
				}
				workers {
					au
				}
				scheduler {
					au
					replicas
				}
				environmentVariablesObjects {
					key
					value
					isSecret
					updatedAt
				}
				webserver {
					url
				}
			}
			workspace {
				id
				organizationId
			}
		}
	}
	`

	SelfQuery = `
	query selfQuery {
		self {
		user {
			roleBindings {
			role
			}
		}
		}
	}
	`

	InternalRuntimeReleases = `
	query RuntimeReleasesQuery($channel: String) {
		runtimeReleases(channel: $channel) {
			version
			channel
		}
	}
	`

	PublicRuntimeReleases = `
	query RuntimeReleasesQuery {
		runtimeReleases {
			version
			channel
		}
	}
	`

	DeploymentHistoryQuery = `
	query deploymentHistory(
		$deploymentId: Id!
		$logCountLimit: Int
		$start: String
		$logLevels: [LogLevel!]
	) {
		deploymentHistory(
		deploymentId: $deploymentId
		logCountLimit: $logCountLimit
		start: $start
		logLevels: $logLevels
		) {
		    schedulerLogs {
			   timestamp
			   raw
			   level
		   }
		}
	}
	`

	GetClusters = `
	query clusters($organizationId: Id) {
		clusters(organizationId: $organizationId) {
			id
			name
			cloudProvider
		}
	}
	`

	GetDeploymentConfigOptions = `
	query deploymentConfigOptions {
	  deploymentConfigOptions {
		components
		astroUnit {
		  cpu
		  memory
		}
		executors
		runtimeReleases {
			version
		}
	  }
	}
  `
	// TODO confirm if this query will work
	GetWorkspace = `
	query GetWorkspace($workspaceId: Id!) {
		workspace(id:$workspaceId) {
			id
			label
			organizationId
		}
	}`
)
