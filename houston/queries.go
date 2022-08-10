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
		$runtimeVersion: String
		$namespace: String
		$config: JSON
		$cloudRole: String
		$dagDeployment: DagDeployment
		$triggererReplicas: Int
	) {
		createDeployment(
			label: $label
			type: $type
			workspaceUuid: $workspaceId
			releaseName: $releaseName
			executor: $executor
		    airflowVersion: $airflowVersion
			runtimeVersion: $runtimeVersion
			namespace: $namespace
			config: $config
			cloudRole: $cloudRole
			dagDeployment: $dagDeployment
			triggerer: {
				replicas: $triggererReplicas
			}
		) {
			id
			type
			label
			releaseName
			version
			airflowVersion
			runtimeVersion
			urls {
				type
				url
			}
			createdAt
			updatedAt
		}
	}`

	DeploymentDeleteRequest = `
	mutation DeleteDeployment(
		$deploymentId: Uuid!
		$deploymentHardDelete: Boolean
		) {
		deleteDeployment(
			deploymentUuid: $deploymentId
			deploymentHardDelete: $deploymentHardDelete
		) {
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
			runtimeVersion
			createdAt
			updatedAt
		}
	}`

	AvailableNamespacesGetRequest = `
	query availableNamespaces {
		availableNamespaces{
			name
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
			runtimeVersion
			desiredRuntimeVersion
			runtimeAirflowVersion
			urls {
				type
				url
			}
		}
	}`

	DeploymentUpdateRequest = `
mutation UpdateDeployment($deploymentId: Uuid!, $payload: JSON!, $executor: ExecutorType, $cloudRole: String, $dagDeployment: DagDeployment, $triggererReplicas: Int) {
		updateDeployment(deploymentUuid: $deploymentId, payload: $payload, executor: $executor, cloudRole: $cloudRole, dagDeployment: $dagDeployment, triggerer:{ replicas: $triggererReplicas }) {
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
			deployInfo {
				current
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

	UpdateDeploymentRuntimeRequest = `
	mutation updateDeploymentRuntime($deploymentUuid: Uuid!, $desiredRuntimeVersion: String!) {
		updateDeploymentRuntime(deploymentUuid: $deploymentUuid, desiredRuntimeVersion: $desiredRuntimeVersion) {
		  	id
			label
			version
			releaseName
		  	runtimeVersion
		  	desiredRuntimeVersion
		  	runtimeAirflowVersion
		}
	}`

	CancelUpdateDeploymentRuntimeRequest = `
	mutation cancelRuntimeUpdate($deploymentUuid: Uuid!) {
		cancelRuntimeUpdate(deploymentUuid: $deploymentUuid) {
			id
			label
			version
			releaseName
			runtimeVersion
			desiredRuntimeVersion
			runtimeAirflowVersion
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
         $serviceAccountUuid: Uuid!
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
	DeploymentServiceAccountsGetRequest = `
  query GetDeploymentServiceAccounts(
	$deploymentUuid: Uuid!
  ){
	deploymentServiceAccounts(
		deploymentUuid: $deploymentUuid
	){
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
  }
  `

	WorkspaceServiceAccountsGetRequest = `
  query GetWorkspaceServiceAccounts(
	$workspaceUuid: Uuid!
  ){
	workspaceServiceAccounts(
		workspaceUuid: $workspaceUuid
	){
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
  }
  `
	// #nosec
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

	WorkspaceGetRequest = `
	query GetWorkspace(
		$workspaceUuid: Uuid!
	){
		workspace(
			workspaceUuid: $workspaceUuid
		){
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
				serviceAccount {
					id
					label
				}
			}
		}
	}
    `

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
				serviceAccount {
					id
					label
				}
			}
		}
	}`

	WorkspacesPaginatedGetRequest = `
	query paginatedWorkspaces(
		$pageSize: Int
		$pageNumber: Int
	  ) {
		  paginatedWorkspaces(
				take: $pageSize
				pageNumber: $pageNumber
		  ) {
			id
			label
			description
			createdAt
			updatedAt
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
	mutation workspaceUpsertUserRole($workspaceUuid: Uuid!, $email: String!, $role: Role!) {
		workspaceUpsertUserRole(
        	workspaceUuid: $workspaceUuid
            email: $email
            role: $role
        )
	}`

	WorkspaceGetUserRequest = `
	query workspaceGetUser($workspaceUuid: Uuid!, $email: String!) {
		workspaceUser(
        	workspaceUuid: $workspaceUuid
                user: { email: $email }
        ) {
		id
		username
		roleBindings {
		  workspace{
			id
		  }
		  role
		}
	}
	}`

	WorkspaceGetUsersRequest = `
	query workspaceListUsers($workspaceUuid: Uuid!) {
		workspaceUsers(
        	workspaceUuid: $workspaceUuid
        ) {
		id
		username
		fullName
		emails {
			address
		}
		roleBindings {
		  workspace{
			id
		  }
		  role
		}
	}
	}`

	WorkspacePaginatedGetUsersRequest = `
	query paginatedWorkspaceUsers(
	  $workspaceUuid: Uuid!,
	  $cursorUuid: Uuid,
	  $take: Int
	) {
		paginatedWorkspaceUsers(
        	workspaceUuid: $workspaceUuid
        	cursor: $cursorUuid
        	take: $take
        ) {
		id
		username
		fullName
		emails {
			address
		}
		roleBindings {
		  workspace{
			id
		  }
		  role
		}
	}
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
			id
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
        	id
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
			airflowVersions
			defaultAirflowImageTag
		}
	}`
	AppVersionRequest = `
	query AppConfig {
		appConfig {
			version
			baseDomain
		}
	}
	`
	AppConfigRequest = `
	query AppConfig {
		appConfig {
			version
			baseDomain
			byoUpdateRegistryHost
			smtpConfigured
			manualReleaseNames
			configureDagDeployment
			nfsMountDagDeployment
			manualNamespaceNames
			hardDeleteDeployment
			triggererEnabled
			featureFlags
		}
	}`

	DeploymentImageUpdateRequest = `
	mutation updateDeploymentImage(
		$releaseName:String!,
		$image:String!,
		$airflowVersion:String,
		$runtimeVersion:String,
	) {
		updateDeploymentImage(
			releaseName:$releaseName,
			image:$image,
			airflowVersion:$airflowVersion,
			runtimeVersion:$runtimeVersion
		) {
			releaseName
			airflowVersion
			runtimeVersion
		}
	}
	`
	GetRuntimeReleases = `
	query runtimeReleases($airflowVersion: String) {
		runtimeReleases(airflowVersion: $airflowVersion) {
		  	version
		  	airflowVersion
		  	airflowDatabaseMigrations
		}
	}`

	TeamGetRequest = `
	query team($teamUuid: Uuid!, $workspaceUuid: Uuid) {
		team(teamUuid: $teamUuid, workspaceUuid: $workspaceUuid) {
			name
			id
			createdAt
			roleBindings {
				id
				role
				workspace {
					id
					label
				}
				deployment {
					id
					label
					releaseName
				}
			}
		}
	}
	`

	TeamGetUsersRequest = `
	query GetTeamUsers(
		$teamUuid: Uuid!
	){
		teamUsers(
			teamUuid: $teamUuid
		){
			username
			id
			emails {
				address
				verified
				primary
			}
			status
		}
	}
	`

	WorkspaceTeamAddRequest = `
	mutation AddWorkspaceTeam(
		$workspaceUuid: Uuid!
		$teamUuid: Uuid!
		$role: Role! = WORKSPACE_VIEWER
		$deploymentRoles: [DeploymentRoles!] = []
	){
		workspaceAddTeam(
			workspaceUuid: $workspaceUuid
			teamUuid: $teamUuid
			role: $role
			deploymentRoles: $deploymentRoles) {
			id
			label
			description
			createdAt
			updatedAt
		}
	}`

	WorkspaceTeamUpdateRequest = `
	mutation workspaceUpdateTeamRole(
		$workspaceUuid: Uuid!
		$teamUuid: Uuid!
		$role: Role!
	){
		workspaceUpdateTeamRole(
      		workspaceUuid: $workspaceUuid
      		teamUuid: $teamUuid
      		role: $role
    	)
	}`

	WorkspaceGetTeamsRequest = `
	query workspaceGetTeams($workspaceUuid: Uuid!) {
		workspaceTeams(
			workspaceUuid: $workspaceUuid
		){
			id
      		name
			roleBindings {
				role
				workspace {
					id
					label
				}
				deployment {
					id
					label
				}
			}
		}
	}
	`

	WorkspaceTeamRemoveRequest = `
	mutation workspaceRemoveTeam(
		$workspaceUuid: Uuid!,
		$teamUuid: Uuid!
	){
		workspaceRemoveTeam(workspaceUuid: $workspaceUuid, teamUuid: $teamUuid) {
			id
			label
		}
	}
	`

	ListTeamsRequest = `
	query paginatedTeams (
		$take: Int
		$cursor: Uuid
		$workspaceUuid: Uuid
	) {
		paginatedTeams(take:$take, cursor:$cursor, workspaceUuid:$workspaceUuid) {
			count
			teams {
				id
				name
			}
		}
	}
	`

	CreateTeamSystemRoleBindingRequest = `
	mutation createTeamSystemRoleBinding (
		$teamUuid: Uuid!
		$role: Role!
	) {
		createTeamSystemRoleBinding(teamUuid:$teamUuid, role:$role) {
			role
		}
	}
	`

	DeleteTeamSystemRoleBindingRequest = `
	mutation deleteTeamSystemRoleBinding (
		$teamUuid: Uuid!
		$role: Role!
	) {
		deleteTeamSystemRoleBinding(teamUuid:$teamUuid, role:$role) {
			role
		}
	}
	`

	DeploymentTeamAddRequest = `
	mutation deploymentAddTeamRole(
		$teamUuid: Uuid!
		$deploymentUuid: Uuid!
		$role: Role! = WORKSPACE_VIEWER
	) {
		deploymentAddTeamRole(
			teamUuid: $teamUuid
			deploymentUuid: $deploymentUuid
			role: $role
		) {
			id
			role
		}
	}`

	DeploymentTeamRemoveRequest = `
	mutation deploymentRemoveTeamRole($deploymentUuid: Uuid!, $teamUuid: Uuid!) {
		deploymentRemoveTeamRole(
			deploymentUuid: $deploymentUuid
			teamUuid: $teamUuid
		) {
			id
		}
	}`

	DeploymentTeamUpdateRequest = `
	mutation deploymentUpdateTeamRole(
		$deploymentUuid: Uuid!
		$teamUuid: Uuid!
		$role: Role!
	) {
		deploymentUpdateTeamRole(
			deploymentUuid: $deploymentUuid
			teamUuid: $teamUuid
			role: $role
		) {
			id
			role
		}
	}`

	DeploymentGetTeamsRequest = `
	query deploymentTeams($deploymentUuid: Uuid!) {
		deploymentTeams(deploymentUuid: $deploymentUuid) {
			id
			name
			roleBindings {
				role
				workspace {
					id
					label
				}
				deployment {
					id
					label
				}
			}
		}
	}`
)
