package houston

// TODO: @adam2k Reorganize based on this issue - https://github.com/astronomer/issues/issues/1991
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
		$releaseName: String
		$workspaceId: Uuid!
		$executor: ExecutorType!
		$airflowVersion: String
		$config: JSON
		$cloudRole: String
	) {
		createDeployment(
			label: $label
			type: $type
			workspaceUuid: $workspaceId
			releaseName: $releaseName
			executor: $executor
		        airflowVersion: $airflowVersion
			config: $config
			cloudRole: $cloudRole
		) {
			id
			type
			label
			releaseName
			version
			airflowVersion
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
			airflowVersion
			createdAt
			updatedAt
		}
	}`

	DeploymentGetRequest = `
	query GetDeployment(
		$id: String!
	) {
		deployment(
			where: {id: $id}
		) {
			id
			airflowVersion
			desiredAirflowVersion
		}
	}`

	DeploymentUpdateRequest = `
	mutation UpdateDeployment($deploymentId: Uuid!, $payload: JSON!, $cloudRole: String) {
		updateDeployment(deploymentUuid: $deploymentId, payload: $payload, cloudRole: $cloudRole) {
			id
			type
			label
			description
			releaseName
			version
			airflowVersion
			workspace {
				id
			}
			createdAt
			updatedAt
		}
	}`

	UpdateDeploymentAirflowRequest = `
	mutation updateDeploymentAirflow($deploymentId: Uuid!, $desiredAirflowVersion: String!) {
		updateDeploymentAirflow(deploymentUuid: $deploymentId, desiredAirflowVersion: $desiredAirflowVersion) {
			id
			label
			version
			releaseName
			airflowVersion
			desiredAirflowVersion
		}
	}`

	// DeploymentUserListRequest return the users for a specific deployment by ID
	DeploymentUserListRequest = `
	query deploymentUsers(
    $deploymentId: Id!
    $user: UserSearch
  ) {
    deploymentUsers(
      deploymentId: $deploymentId
      user: $user
    ) {
      id
      fullName
      username
      roleBindings {
        role
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
		$userId: Uuid!
	  ) {
		workspaceRemoveUser(
		  workspaceUuid: $workspaceId
		  userUuid: $userId
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

	DeploymentInfoRequest = `
	query DeploymentInfo {
		deploymentConfig {
			airflowImages {
				version
				tag
			}
			airflowVersions
			defaultAirflowImageTag
		}
	}`
	AppConfigRequest = `
	query AppConfig {
		appConfig {
			version
			baseDomain
			smtpConfigured
			manualReleaseNames
		}
	}`
)
