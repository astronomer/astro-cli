package houston

// TODO: @adam2k Reorganize based on this issue - https://github.com/astronomer/issues/issues/1991
var (
	// DeploymentUserAddRequest Mutation for AddDeploymentUser
	DeploymentUserAddRequest = `
	mutation AddDeploymentUser(
		$userId: Uuid
		$email: String!
		$deploymentId: Uuid!
		$role: Role!
	) {
		deploymentAddUserRole(
			userUuid: $userId
			email: $email
			deploymentUuid: $deploymentId
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
)
