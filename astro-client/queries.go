package astro

var (
	WorkspaceDeploymentsGetRequest = `
	query WorkspaceDeployments(
		$organizationId: Id!, $workspaceId: Id
	) {
		deployments(organizationId: $organizationId, workspaceId: $workspaceId) {
			id
			label
			description
			releaseName
			dagDeployEnabled
			apiKeyOnlyDeployments
			schedulerSize
			type
			isHighAvailability
			cluster {
				id
				name
				cloudProvider
				providerAccount
				region
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
				astroMachine
				isDefault
				nodePoolId
				podCpu
				podRam
				workerConcurrency
				minWorkerCount
				maxWorkerCount
			}
			createdAt
			updatedAt
			alertEmails
			status
			runtimeRelease {
				version
				airflowVersion
			}
			deploymentSpec {
				executor
				image {
					tag
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
				label
				organizationId
				label
			}
		}
	}
	`

	GetDeployment = `
	query GetDeployment($deploymentId: Id!) {
		deployment(id: $deploymentId) {
			id
			label
			description
			releaseName
			cluster {
				id
				name
				nodePools {
					id
					isDefault
					nodeInstanceType
					maxNodeCount
				}
			}
			createdAt
			status
			dagDeployEnabled
			schedulerSize
			type
			isHighAvailability
			runtimeRelease {
				version
				airflowVersion
			}
			deploymentSpec {
				executor
				image {
					tag
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
				label
				organizationId
			}
			workerQueues {
				id
				name
				astroMachine
				isDefault
				nodePoolId
				workerConcurrency
				minWorkerCount
				maxWorkerCount
			}
		}
	}`

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
		astroMachines {
			concurrentTasks
			concurrentTasksMax
			cpu
			memory
			nodePoolType
			storageSize
			type
		}
		defaultAstroMachine {
			concurrentTasks
			concurrentTasksMax
			cpu
			memory
			nodePoolType
			storageSize
			type
		}
		defaultSchedulerSize {
			cpu
			memory
			size
		}
		schedulerSizes {
			cpu
			memory
			size
		}
	  }
	}
  `

	GetDeploymentConfigOptionsWithOrganization = `
	query deploymentConfigOptions($organizationId: Id!) {
	  deploymentConfigOptions(organizationId: $organizationId) {
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
		astroMachines {
			concurrentTasks
			concurrentTasksMax
			cpu
			memory
			nodePoolType
			storageSize
			type
		}
		defaultAstroMachine {
			concurrentTasks
			concurrentTasksMax
			cpu
			memory
			nodePoolType
			storageSize
			type
		}
		defaultSchedulerSize {
			cpu
			memory
			size
		}
		schedulerSizes {
			cpu
			memory
			size
		}
	  }
	}
  `

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
