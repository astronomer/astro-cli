package houston

var (
	AuthConfigGetRequest = `
	query GetAuthConfig($redirect: String) {
		authConfig(redirect: $redirect) {
			localEnabled
			publicSignup
			initialSignup
			providers {
				name
        		displayName
				url
      		}
		}
	}`

	DeploymentCreateRequest = `
	mutation CreateDeployment(
		$label: String!
		$type: String = "airflow"
		$workspaceId: Uuid!
		$config: JSON!
	) {
		createDeployment(
			label: $label
			type: $type
			workspaceUuid: $workspaceId
            config: $config
		) {
			id
			type
			label
			releaseName
			version
			createdAt
			updatedAt
		}
	}`

	DeploymentDeleteRequest = `
	mutation DeleteDeployment($deploymentId: Uuid!) {
		deleteDeployment(deploymentUuid: $deploymentId) {
			id
			type
			label
			description
			releaseName
			version
			workspace {
				id
			}
			createdAt
			updatedAt
		}
	}`

	DeploymentsGetRequest = `
	query GetDeployment(
		$deploymentId: Uuid
		$workspaceId: Uuid
		$releaseName: String
	) {
		deployments(
			deploymentUuid: $deploymentId
			workspaceUuid: $workspaceId
			releaseName: $releaseName
		) {
			id
			type
			label
			releaseName
			workspace {
				id
			}
			deployInfo {
				latest
				next
			}
			version
			createdAt
			updatedAt
		}
	}`

	DeploymentUpdateRequest = `
	mutation UpdateDeployment($deploymentId: Uuid!, $payload: JSON!) {
		updateDeployment(deploymentUuid: $deploymentId, payload: $payload) {
			id
			type
			label
			description
			releaseName
			version
			workspace {
				id
			}
			createdAt
			updatedAt
		}
	}`

	ServiceAccountCreateRequest = `
		mutation CreateServiceAccount(
			$entityId: Uuid!
			$label: String!
			$category: String
			$entityType: EntityType!
		) {
			createServiceAccount(
				entityUuid: $entityId
				label: $label
				category: $category
				entityType: $entityType
			) {
				id
				apiKey
				label
				category
				entityType
				entityUuid
				active
				createdAt
				updatedAt
				lastUsedAt
			}
		}`

	ServiceAccountDeleteRequest = `
	mutation DeleteServiceAccount($serviceAccountId: Uuid!) {
		deleteServiceAccount(serviceAccountUuid: $serviceAccountId) {
					id
					apiKey
					label
					category
					entityType
					entityUuid
					active
					createdAt
					updatedAt
					lastUsedAt
		}
	}`

	ServiceAccountsGetRequest = `
	query GetServiceAccount(
		$serviceAccountId: Uuid
		$entityId: Uuid
		$entityType: EntityType!
	) {
		serviceAccounts(
			serviceAccountUuid: $serviceAccountId
			entityType: $entityType
			entityUuid: $entityId
		) {
			id
			apiKey
			label
			category
			entityType
			entityUuid
			active
			createdAt
			updatedAt
			lastUsedAt
		}
	}`

	TokenBasicCreateRequest = `
	mutation createBasicToken($identity: String, $password: String!) {
		createToken(identity: $identity, password: $password) {
			user {
				id
				fullName
				username
				status
				createdAt
				updatedAt
			}
			token {
				value
			}
		}
	}`

	UserCreateRequest = `
	mutation CreateUser(
		$email: String!
		$password: String!
		$username: String
		$inviteToken: String
	) {
		createUser(
			email: $email
			password: $password
			username: $username
			inviteToken: $inviteToken
		) {
			user {
				id
				username
				status
				createdAt
				updatedAt
			}
			token {
				value
			}
		}
	}`

	WorkspacesGetRequest = `
	query GetWorkspaces($workspaceId: Uuid, $label: String, $userId: Uuid) {
		workspaces(
			workspaceUuid: $workspaceId
			label: $label
			userUuid: $userId
		) {
			id
			label
			description
			createdAt
			updatedAt
			roleBindings {
        		role
  				user {
                  id
                  username
        		}
      		}
		}
	}`

	WorkspaceCreateRequest = `
	mutation CreateWorkspace($label: String!, $description: String = "N/A") {
		createWorkspace(label: $label, description: $description) {
			id
			label
			description
			createdAt
			updatedAt
		}
	}`

	WorkspaceDeleteRequest = `
	mutation DeleteWorkspace($workspaceId: Uuid!) {
		deleteWorkspace(workspaceUuid: $workspaceId) {
			id
			label
			description
			createdAt
			updatedAt
		}
	}`

	WorkspaceUpdateRequest = `
	mutation UpdateWorkspace($workspaceId: Uuid!, $payload: JSON!) {
		updateWorkspace(workspaceUuid: $workspaceId, payload: $payload) {
			id
			label
			description
			deploymentCount
			createdAt
			updatedAt
		}
	}`

	WorkspaceUserAddRequest = `
	mutation AddWorkspaceUser($workspaceId: Uuid!, $email: String!, $role: Role!) {
		workspaceAddUser(workspaceUuid: $workspaceId, email: $email, role: $role) {
			id
			label
			description
			users {
				id
				username
				roleBindings {
          			role
        		}
			}
			createdAt
			updatedAt
		}
	}`

	WorkspaceUserUpdateRequest = `
	mutation workspaceUpdateUserRole($workspaceUuid: Uuid!, $email: String!, $role: Role!) {
		workspaceUpdateUserRole(
        	workspaceUuid: $workspaceUuid
            email: $email
            role: $role
        )
	}`

	WorkspaceUserRemoveRequest = `
	mutation RemoveWorkspaceUser(
		$workspaceId: Uuid!
		$userId: Uuid
		$email: String
	  ) {
		workspaceRemoveUser(
		  workspaceUuid: $workspaceId
		  userUuid: $userId
		  email: $email
		) {
		  id
		  label
		  description
		  users {
			id
			username
		  }
		  createdAt
		  updatedAt
		}
	  }`
	DeploymentLogsGetRequest = `
	query GetLogs(
		$deploymentId: Uuid!
		$component: String
		$timestamp: DateTime
		$search: String
	) {
		logs(
			deploymentUuid: $deploymentId
			component: $component
			timestamp: $timestamp
			search: $search
		) {
			id: uuid
			createdAt: timestamp
			log: message
		}
	}`
	DeploymentLogsSubscribeRequest = `
    subscription log(
		$deploymentId: Uuid!
		$component: String
		$timestamp: DateTime
		$search: String
    ) {
      	log(
			deploymentUuid: $deploymentId
			component: $component
			timestamp: $timestamp
			search: $search
      ) {
        	id: uuid
        	createdAt: timestamp
        	log: message
      }
    }`
)
