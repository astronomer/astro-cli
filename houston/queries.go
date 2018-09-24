package houston

// "github.com/sirupsen/logrus"

var (

	// authConfigGetRequest = `
	// query GetAuthConfig {
	// 	authConfig(redirect: "") {
	// 	  localEnabled
	// 	  googleEnabled
	// 	  githubEnabled
	// 	  auth0Enabled
	// 	  googleOAuthUrl
	// 	}
	// 	}`

	authConfigGetRequest = `
	query GetAuthConfig($redirect: String!) {
		authConfig(redirect: $redirect) {
			localEnabled
			googleEnabled
			githubEnabled
			auth0Enabled
			googleOAuthUrl
		}
	}`

	// deploymentCreateRequest = `
	// mutation CreateDeployment(
	// 	$label: String!,
	// 	$type: String! = "airflow",
	// 	workspaceUuid: Uuid) {
	// 	createDeployment(
	// 		label: "%s",
	// 		type: "airflow",
	// 		workspaceUuid: "%s"
	// 	) {
	// 		uuid
	// 		type
	// 		label
	// 		releaseName
	// 		version
	// 		createdAt
	// 		updatedAt
	// 	}
	// }`

	deploymentCreateRequest = `
	mutation CreateDeployment(
		$label: String!
		$type: String = "airflow"
		$version: String = "1.9.0"
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

	// deploymentDeleteRequest = `
	// mutation DeleteDeployment {
	// 	deleteDeployment(deploymentUuid: "%s") {
	// 		uuid
	// 		type
	// 		label
	// 		releaseName
	// 		version
	// 		createdAt
	// 		updatedAt
	// 	}
	// }`

	deploymentDeleteRequest = `
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

	// TODO replaced by deploymentsGetRequest
	// deploymentGetRequest = `
	// query GetDeployment {
	//   deployments(
	// 		deploymentUuid: "%s"
	// 	) {
	// 		uuid
	// 		type
	// 		label
	// 		releaseName
	// 		version
	// 		createdAt
	// 		updatedAt
	//   }
	// }`

	deploymentsGetRequest = `
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

	// TODO replaced by deploymentsGetRequest
	// deploymentsGetAllRequest = `
	// query GetAllDeployments {
	//   deployments {
	// 	uuid
	// 	type
	// 	label
	// 	releaseName
	// 	workspace {
	// 		uuid
	// 	}
	// 	deployInfo {
	// 		latest
	// 		next
	// 	}
	// 	version
	// 	createdAt
	// 	updatedAt
	//   }
	// }`

	// deploymentUpdateRequest = `
	// mutation UpdateDeplomyent {
	// 	updateDeployment(deploymentUuid:"%s",
	// 	  payload: %s
	// 	) {
	// 	  uuid
	// 	  type
	// 	  label
	// 	  releaseName
	// 	  version
	// 	}
	//   }`

	deploymentUpdateRequest = `
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

	// serviceAccountCreateRequest = `
	// mutation CreateServiceAccount {
	// 	createServiceAccount(
	// 		entityUuid: "%s",
	// 		label: "%s",
	// 		category: "%s",
	// 		entityType: %s
	// 	) {
	// 		uuid
	// 		apiKey
	// 		label
	// 		category
	// 		entityType
	// 		entityUuid
	// 		active
	// 		createdAt
	// 		updatedAt
	// 		lastUsedAt
	// 	}
	// 	}`

	serviceAccountCreateRequest = `
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

	// serviceAccountDeleteRequest = `
	// mutation DeleteServiceAccount {
	// 	deleteServiceAccount(
	// 	serviceAccountUuid:"%s"
	// 	) {
	// 	  uuid
	// 	  label
	// 	  category
	// 	  entityType
	// 	  entityUuid
	// 	}
	//   }`

	serviceAccountDeleteRequest = `
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

	// serviceAccountsGetRequest = `
	// query GetServiceAccount {
	// 	serviceAccounts(
	// 		entityType:%s,
	//     entityUuid:"%s"
	// 	) {
	//     uuid
	//     apiKey
	//     label
	//     category
	//     entityType
	//     entityUuid
	//     active
	//     createdAt
	//     updatedAt
	//     lastUsedAt
	// 	}
	//   }`

	serviceAccountsGetRequest = `
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

	// tokenBasicCreateRequest = `
	// mutation createBasicToken {
	//   createToken(
	// 	  identity:"%s",
	// 	  password:"%s"
	// 	) {
	// 		user {
	// 			uuid
	// 			username
	// 			status
	// 			createdAt
	// 			updatedAt
	// 		}
	// 		token {
	// 			value
	// 		}
	// 	}
	// }`

	tokenBasicCreateRequest = `
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

	// userCreateRequest = `
	// mutation CreateUser {
	// 	createUser(
	// 		email: "%s",
	// 		password: "%s"
	// 	) {
	// 		user {
	// 			uuid
	// 			username
	// 			status
	// 			createdAt
	// 			updatedAt
	// 		}
	// 		token {
	// 			value
	// 		}
	// 	}
	// }`

	userCreateRequest = `
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

	// userGetAllRequest = `
	// query GetUsers {
	// 	users {
	// 		uuid
	// 		emails {
	// 			address
	// 			verified
	// 			primary
	// 			createdAt
	// 			updatedAt
	// 		}
	// 		fullName
	// 		username
	// 		status
	// 		createdAt
	// 		updatedAt
	// 	}
	// }`

	userGetAllRequest = `
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

	workspaceAllGetRequest = `
	query GetWorkspaces {
		workspaces {
			uuid
			label
			description
			active
			createdAt
			updatedAt
		}
	}`

	// workspaceGetRequest = `
	// query GetWorkspaces {
	// 	workspaces(workspaceUuid:"%s") {
	// 		uuid
	// 		label
	// 		description
	// 		active
	// 		createdAt
	// 		updatedAt
	// 	}
	// }`

	workspaceGetRequest = `
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

	// workspaceCreateRequest = `
	// mutation CreateWorkspace {
	// 	createWorkspace(
	// 		label: "%s",
	// 		description: "%s"
	// 	) {
	// 		uuid
	// 		label
	// 		description
	// 		active
	// 		createdAt
	// 		updatedAt
	// 	}
	// }`

	workspaceCreateRequest = `
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

	// workspaceDeleteRequest = `
	// mutation DeleteWorkspace {
	// 	deleteWorkspace(workspaceUuid: "%s") {
	// 		uuid
	// 		label
	// 		description
	// 		active
	// 		createdAt
	// 		updatedAt
	// 	}
	// }`

	workspaceDeleteRequest = `
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

	// workspaceUpdateRequest = `
	// mutation UpdateWorkspace {
	// 	updateWorkspace(workspaceUuid:"%s",
	// 	  payload: %s
	// 	) {
	// 	  uuid
	// 	  description
	// 	  label
	// 	  active
	// 	}
	//   }`

	workspaceUpdateRequest = `
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

	// workspaceUserAddRequest = `
	// mutation AddWorkspaceUser {
	// 	workspaceAddUser(
	// 		workspaceUuid: "%s",
	// 		email: "%s",
	// 	) {
	// 		uuid
	// 		label
	// 		description
	// 		active
	// 		users {
	// 			uuid
	// 			username
	// 		}
	// 		createdAt
	// 		updatedAt
	// 	}
	// }`

	workspaceUserAddRequest = `
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

	// workspaceUserRemoveRequest = `
	// mutation RemoveWorkspaceUser {
	// 	workspaceRemoveUser(
	// 		workspaceUuid: "%s",
	// 		userUuid: "%s",
	// 	) {
	// 		uuid
	// 		label
	// 		description
	// 		active
	// 		users {
	// 			uuid
	// 			username
	// 		}
	// 		createdAt
	// 		updatedAt
	// 	}
	// }`

	workspaceUserRemoveRequest = `
	mutation RemoveWorkspaceUser($workspaceUuid: Uuid!, $userUuid: Uuid!) {
		workspaceRemoveUser(workspaceUuid: $workspaceUuid, userUuid: $userUuid) {
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
