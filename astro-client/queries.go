package astro

// TODO: @adam2k Reorganize based on this issue - https://github.com/astronomer/issues/issues/1991
var (
	WorkspacesGetRequest = `
	query GetWorkspaces($organizationId: Id!) {
		workspaces(organizationId: $organizationId) {
			id
			label
			organizationId
		}
	}`

	WorkspaceDeploymentsGetRequest = `
	query WorkspaceDeployments(
		$organizationId: Id!, $workspaceId: Id
	) {
		deployments(organizationId: $organizationId, workspaceId: $workspaceId) {
			id
			label
			releaseName
			cluster {
				id
			}
			cluster {
				id
				name
				cloudProvider
				nodePools {
					id
					isDefault
					nodeInstanceType
					createdAt
				}
			}
			workerQueues {
				id
				name
				isDefault
				nodePoolId
				podCpu
				podRam
				workerConcurrency
				minWorkerCount
				maxWorkerCount
			}
			createdAt
			status
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

	GetDeployment = `
	query GetDeployment($deploymentId: Id!) {
		deployment(id: $deploymentId) {
			id
			label
			releaseName
			cluster {
				id
			}
			createdAt
			status
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
	}`

	SelfQuery = `
	query selfQuery {
		self {
		user {
			roleBindings {
			role
			}
		}
		authenticatedOrganizationId
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
	query clusters($organizationId: Id!) {
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
			channel
			version
		}
	  }
	}
  `
	GetWorkspace = `
	query GetWorkspace($workspaceId: Id!) {
		workspace(id:$workspaceId) {
			id
			label
			organizationId
		}
	}`

	GetWorkerQueueOptions = `
	query workerQueueOptions {
		workerQueueOptions {
			minWorkerCount {
			  floor
			  ceiling
			  default
			}
			maxWorkerCount {
			  floor
			  ceiling
			  default
			}
			workerConcurrency {
			  floor
			  ceiling
			  default
			}
		}
	}`
)
