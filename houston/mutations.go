package houston

// TODO: @adam2k Reorganize based on this issue - https://github.com/astronomer/issues/issues/1991
var (
	// DeploymentUserAddRequest Mutation for AddDeploymentUser
	DeploymentUserAddRequest = `
	mutation AddDeploymentUser(
		$userId: Id
		$email: String!
		$deploymentId: Id!
		$role: Role!
	) {
		deploymentAddUserRole(
			userId: $userId
			email: $email
			deploymentId: $deploymentId
			role: $role
		) {
			id
			user {
				username
			}
			role
			deployment {
				id
				releaseName
			}
		}
	}
	`

	// DeploymentUserDeleteRequest Mutation for AddDeploymentUser
	DeploymentUserDeleteRequest = `
	mutation DeleteDeploymentUser(
		$userId: Id
		$email: String!
		$deploymentId: Id!
	) {
		deploymentRemoveUserRole(
			userId: $userId
			email: $email
			deploymentId: $deploymentId
		) {
			id
			role
		}
	}
	`
)
