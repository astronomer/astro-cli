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

	DeploymentDelete = `
	mutation deploymentDelete(
		$input: DeploymentDeleteInput!
	  ) {
		deploymentDelete(
			input: $input
		) {
		  id
		}
	}
	`

	DeploymentCreate = `
	mutation deploymentCreate(
		$input: DeploymentCreateInput
	  ) {
		deploymentCreate (
		  input: $input
		){
			id
			label
			releaseName
			orchestrator {
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

	DeploymentUpdate = `
	mutation deploymentUpdate(
		$input: DeploymentUpdateInput
	  ) {
		deploymentUpdate(
			input: $input
		) {
			id
			label
			releaseName
			orchestrator {
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
