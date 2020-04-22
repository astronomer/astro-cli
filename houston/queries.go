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
		$releaseName: String!
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
			urls {
				type
				url
			}
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
		$workspaceId: Uuid!
		$releaseName: String
	) {
		workspaceDeployments(
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
				nextCli
				current
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
			$role: Role!
		) {
			createServiceAccount(
				entityUuid: $entityId
				label: $label
				category: $category
				entityType: $entityType
				role: $role
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

	CreateDeploymentServiceAccountRequest = `
	mutation createDeploymentServiceAccount(
		$label: String!,
		$category: String,
		$deploymentUuid: Uuid!,
		$role: Role!
  	) {
		createDeploymentServiceAccount(
		  label: $label,
		  category: $category,
		  deploymentUuid: $deploymentUuid,
		  role: $role
    	) {
		    id
		    label
		    apiKey
		    entityType
		    deploymentUuid
		    category
		    active
		    lastUsedAt
		    createdAt
		    updatedAt
    	}
  	}`

	CreateWorkspaceServiceAccountRequest = `
	mutation createWorkspaceServiceAccount(
		$label: String!,
		$category: String,
		$workspaceUuid: Uuid!,
		$role: Role!
	) {
		createWorkspaceServiceAccount(
		  label: $label,
		  category: $category,
		  workspaceUuid: $workspaceUuid,
		  role: $role
		) {
			id
			label
			apiKey
			entityType
			workspaceUuid
			category
			active
			lastUsedAt
			createdAt
			updatedAt
		}
    }`

	DeploymentServiceAccountDeleteRequest = `
	mutation deleteDeploymentServiceAccount(
         $serviceAccountId: Uuid!
         $deploymentUuid: Uuid!
    ) {
		deleteDeploymentServiceAccount(
          serviceAccountUuid: $serviceAccountUuid
          deploymentUuid: $deploymentUuid
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

	WorkspaceServiceAccountDeleteRequest = `
	mutation deleteWorkspaceServiceAccount(
          $serviceAccountUuid: Uuid!
          $workspaceUuid: Uuid!
    ) {
        deleteWorkspaceServiceAccount(
          serviceAccountUuid: $serviceAccountUuid
          workspaceUuid: $workspaceUuid
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
	query GetWorkspaces {
		workspaces {
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
	DeploymentInfoRequest = `
	query DeploymentInfo {
		deploymentConfig {
			airflowImages {
				version
				tag
		}
		defaultAirflowImageTag
		}
	}`
)
