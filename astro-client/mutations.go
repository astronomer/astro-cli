package astro

var (
	ImageCreate = `
	mutation ImageCreate($imageCreateInput: ImageCreateInput!) {
		imageCreate(input: $imageCreateInput) {
			id
			deploymentId
			tag
		}
	}
	`

	ImageDeploy = `
	mutation ImageDeploy($imageDeployInput: ImageDeployInput!) {
		imageDeploy(input: $imageDeployInput) {
			id
			tag
			repository
		}
	}
	`

	DeleteDeployment = `
	mutation deleteDeployment(
		$input: DeleteDeploymentInput!
	  ) {
		deleteDeployment(
			input: $input
		) {
		  id
		}
	}
	`

	CreateDeployment = `
	mutation createDeployment(
		$input: CreateDeploymentInput!
	  ) {
		createDeployment (
		  input: $input
		){
			id
			label
			releaseName
			cluster {
				id
			}
			runtimeRelease {
				version
				airflowVersion
			}
			deploymentSpec {
				image {
					tag
				}
				webserver {
					url
				}
			}
		}
	}
	`

	UpdateDeployment = `
	mutation updateDeployment(
		$input: UpdateDeploymentInput!
	  ) {
		updateDeployment(
			input: $input
		) {
			id
			label
			releaseName
			cluster {
				id
			}
			runtimeRelease {
				version
				airflowVersion
			}
			deploymentSpec {
				image {
					tag
				}
			}
	  	}
	}
	`
	DeploymentVariablesCreate = `
	mutation deploymentVariablesUpdate(
	  $input: EnvironmentVariablesInput!
	) {
	  deploymentVariablesUpdate(
		input: $input
	  ) {
		key
		value
		isSecret
		updatedAt
	  }
	}
  `
	CreateUserInvite = `
  	mutation createUserInvite($input: CreateUserInviteInput!) {
	  createUserInvite(input: $input) {
			userId
			organizationId
			oauthInviteId
			expiresAt
	  }
  	}
  `
	DagDeploymentInitiate = `
	mutation initiateDagDeployment($input: InitiateDagDeploymentInput!) {
		initiateDagDeployment(input: $input) {
			id
			dagUrl
		}
	}
  `
	ReportDagDeploymentStatus = `
	mutation reportDagDeploymentStatus($input: ReportDagDeploymentStatusInput!) {
		reportDagDeploymentStatus(input: $input) {
			id
			deploymentId
			action
			versionId
			status
			message
			createdAt
			initiatorId
			initiatorType
		}
	}
  `
)
