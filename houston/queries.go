package houston

// "github.com/sirupsen/logrus"

var (
	AuthConfigGetRequest = `
	query GetAuthConfig($redirect: String) {
		authConfig(redirect: $redirect) {
			localEnabled
			googleEnabled
			githubEnabled
			auth0Enabled
			googleOAuthUrl
		}
	}`

	DeploymentCreateRequest = `
	mutation CreateDeployment(
		$label: String!
		$type: String = "airflow"
		$version: String = "0.7.5"
		$workspaceUuid: Uuid!
	) {
		createDeployment(
			label: $label
			type: $type
			version: $version
			workspaceUuid: $workspaceUuid
		) {
			uuid
			type
			label
			releaseName
			version
			createdAt
			updatedAt
		}
	}`

	DeploymentDeleteRequest = `
	mutation DeleteDeployment($deploymentUuid: Uuid!) {
		deleteDeployment(deploymentUuid: $deploymentUuid) {
			uuid
			type
			label
      description
			releaseName
			version
      workspace {
        uuid
      }
			createdAt
			updatedAt
		}
	}`

	DeploymentsGetRequest = `
	query GetDeployment(
		$deploymentUuid: Uuid
		$workspaceUuid: Uuid
		$releaseName: String
	) {
		deployments(
			deploymentUuid: $deploymentUuid
			workspaceUuid: $workspaceUuid
			releaseName: $releaseName
		) {
			uuid
			type
			label
			releaseName
			workspace {
				uuid
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
	mutation UpdateDeplomyent($deploymentUuid: Uuid!, $payload: JSON!) {
		updateDeployment(deploymentUuid: $deploymentUuid, payload: $payload) {
			uuid
			type
			label
			description
			releaseName
			version
			workspace {
				uuid
			}
			createdAt
			updatedAt
		}
	}`

	ServiceAccountCreateRequest = `
		mutation CreateServiceAccount(
			$entityUuid: Uuid!
			$label: String!
			$category: String
			$entityType: EntityType!
		) {
			createServiceAccount(
				entityUuid: $entityUuid
				label: $label
				category: $category
				entityType: $entityType
			) {
				uuid
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
	mutation DeleteServiceAccount($serviceAccountUuid: Uuid!) {
		deleteServiceAccount(serviceAccountUuid: $serviceAccountUuid) {
					uuid
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
		$serviceAccountUuid: Uuid
		$entityUuid: Uuid
		$entityType: EntityType!
	) {
		serviceAccounts(
			serviceAccountUuid: $serviceAccountUuid
			entityType: $entityType
			entityUuid: $entityUuid
		) {
			uuid
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
				uuid
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
				uuid
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

	UserGetAllRequest = `
	query GetUsers($userUuid: Uuid, $username: String, $email: String) {
		users(userUuid: $userUuid, username: $username, email: $email) {
			uuid
			emails {
				address
				verified
				primary
				createdAt
				updatedAt
			}
			fullName
			username
			status
			createdAt
			updatedAt
		}
	}`

	WorkspacesGetRequest = `
	query GetWorkspaces($workspaceUuid: Uuid, $label: String, $userUuid: Uuid) {
		workspaces(
			workspaceUuid: $workspaceUuid
			label: $label
			userUuid: $userUuid
		) {
			uuid
			label
			description
			active
			createdAt
			updatedAt
		}
	}`

	WorkspaceCreateRequest = `
	mutation CreateWorkspace($label: String!, $description: String = "N/A") {
		createWorkspace(label: $label, description: $description) {
			uuid
			label
			description
			active
			createdAt
			updatedAt
		}
	}`

	WorkspaceDeleteRequest = `
	mutation DeleteWorkspace($workspaceUuid: Uuid!) {
		deleteWorkspace(workspaceUuid: $workspaceUuid) {
			uuid
			label
			description
			active
			createdAt
			updatedAt
		}
	}`

	WorkspaceUpdateRequest = `
	mutation UpdateWorkspace($workspaceUuid: Uuid!, $payload: JSON!) {
		updateWorkspace(workspaceUuid: $workspaceUuid, payload: $payload) {
			uuid
			label
			description
			active
			deploymentCount
			createdAt
			updatedAt
		}
	}`

	WorkspaceUserAddRequest = `
	mutation AddWorkspaceUser($workspaceUuid: Uuid!, $email: String!) {
		workspaceAddUser(workspaceUuid: $workspaceUuid, email: $email) {
			uuid
			label
			description
			active
			users {
				uuid
				username
			}
			createdAt
			updatedAt
		}
	}`

	WorkspaceUserRemoveRequest = `
	mutation RemoveWorkspaceUser(
		$workspaceUuid: Uuid!
		$userUuid: Uuid
		$email: String
	  ) {
		workspaceRemoveUser(
		  workspaceUuid: $workspaceUuid
		  userUuid: $userUuid
		  email: $email
		) {
		  uuid
		  label
		  description
		  active
		  users {
			uuid
			username
		  }
		  createdAt
		  updatedAt
		}
	  }`
)
