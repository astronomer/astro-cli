package houston

// "github.com/sirupsen/logrus"

var (
	CreateDeploymentTest = `
mutation CreateDeployment(
  $label: String!,
  $type: String = "airflow", 
  $version: String = "1.9.0", 
  $workspaceUuid: Uuid!) {
    createDeployment(
      label: $label,
      type: $type,
      version: $version,
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

	authConfigGetRequest = `
	query GetAuthConfig {
		authConfig(redirect: "") {
		  localEnabled
		  googleEnabled
		  githubEnabled
		  auth0Enabled
		  googleOAuthUrl
		}
	  }`

	deploymentCreateRequest = `
	mutation CreateDeployment(
		$label: String!,
		$type: String! = "airflow",
		workspaceUuid: Uuid) {
		createDeployment(	
			label: "%s",
			type: "airflow",
			workspaceUuid: "%s"
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

	deploymentDeleteRequest = `
	mutation DeleteDeployment {
		deleteDeployment(deploymentUuid: "%s") {
			uuid
			type
			label
			releaseName
			version
			createdAt
			updatedAt
		}
	}`

	deploymentGetRequest = `
	query GetDeployment {
	  deployments(
			deploymentUuid: "%s"
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

	deploymentsGetRequest = `
	query GetDeployments {
	  deployments(workspaceUuid: "%s") {
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

	deploymentsGetAllRequest = `
	query GetAllDeployments {
	  deployments {
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

	deploymentUpdateRequest = `
	mutation UpdateDeplomyent {
		updateDeployment(deploymentUuid:"%s",
		  payload: %s
		) {
		  uuid
		  type
		  label
		  releaseName
		  version
		}
	  }`

	serviceAccountCreateRequest = `
	mutation CreateServiceAccount {
		createServiceAccount(
			entityUuid: "%s",
			label: "%s",
			category: "%s",
			entityType: %s
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

	serviceAccountDeleteRequest = `
	mutation DeleteServiceAccount {
		deleteServiceAccount(
		serviceAccountUuid:"%s"
		) {
		  uuid
		  label
		  category
		  entityType
		  entityUuid
		}
	  }`

	serviceAccountsGetRequest = `
	query GetServiceAccount {
		serviceAccounts(
			entityType:%s,
      entityUuid:"%s"
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

	tokenBasicCreateRequest = `
	mutation createBasicToken {
	  createToken(
		  identity:"%s",
		  password:"%s"
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

	userCreateRequest = `
	mutation CreateUser {
		createUser(
			email: "%s",
			password: "%s"
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

	userGetAllRequest = `
	query GetUsers {
		users {
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

	workspaceGetRequest = `
	query GetWorkspaces {
		workspaces(workspaceUuid:"%s") {
			uuid
			label
			description
			active
			createdAt
			updatedAt
		}
	}`

	workspaceCreateRequest = `
	mutation CreateWorkspace {
		createWorkspace(
			label: "%s",
			description: "%s"
		) {
			uuid
			label
			description
			active
			createdAt
			updatedAt
		}
	}`

	workspaceDeleteRequest = `
	mutation DeleteWorkspace {
		deleteWorkspace(workspaceUuid: "%s") {
			uuid
			label
			description
			active
			createdAt
			updatedAt
		}
	}`

	workspaceUpdateRequest = `
	mutation UpdateWorkspace {
		updateWorkspace(workspaceUuid:"%s",
		  payload: %s
		) {
		  uuid
		  description
		  label
		  active
		}
	  }`

	workspaceUserAddRequest = `
	mutation AddWorkspaceUser {
		workspaceAddUser(
			workspaceUuid: "%s",
			email: "%s",
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

	workspaceUserRemoveRequest = `
	mutation RemoveWorkspaceUser {
		workspaceRemoveUser(
			workspaceUuid: "%s",
			userUuid: "%s",
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
