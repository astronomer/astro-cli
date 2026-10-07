package workerqueue

import (
	"fmt"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
)

// ActionDeleted is a delete's Result action. A create's is "created" and an
// update's "updated", the past tense of the command.
const ActionDeleted = "deleted"

// Result is what `astro deployment worker-queue create`, `update` or
// `delete` did, as they publish it under --output json: the Deployment it
// was done to, and the queue as the change left it, or, for a delete, as it
// was. The Deployment's keys are the ones its other shapes use
// (deployment_id, workspace_id), and action is created, updated or deleted.
type Result struct {
	DeploymentID   string `json:"deployment_id"`
	DeploymentName string `json:"deployment_name"`
	WorkspaceID    string `json:"workspace_id"`
	Action         string `json:"action"`
	WorkerQueue    Queue  `json:"worker_queue"`
}

// Queue is a worker queue, under the keys `astro deployment inspect` gives
// one in worker_queues. worker_type is the Astro machine (A5, A10, ...), or
// on Hybrid the node pool's instance type; it is left out only for a Hybrid
// queue a delete removed, whose node pool the delete does not look up.
type Queue struct {
	Name              string `json:"name"`
	IsDefault         bool   `json:"is_default"`
	WorkerType        string `json:"worker_type,omitempty"`
	MinWorkerCount    int    `json:"min_worker_count"`
	MaxWorkerCount    int    `json:"max_worker_count"`
	WorkerConcurrency int    `json:"worker_concurrency"`
}

// errNoDeployment is the failure of a queue change in a Workspace with no
// Deployment to make it on. It used to be a note, with exit 0.
func errNoDeployment(ws string) error {
	return fmt.Errorf("%s %s", deployment.NoDeploymentInWSMsg, ws)
}

// nodePoolInstanceType is the instance type of the node pool with id, or ""
// when nodePools has none.
func nodePoolInstanceType(id string, nodePools []astrov1.NodePool) string {
	for i := range nodePools {
		if nodePools[i].Id == id {
			return nodePools[i].NodeInstanceType
		}
	}
	return ""
}
