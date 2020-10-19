package houston

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
