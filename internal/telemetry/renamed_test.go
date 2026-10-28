package telemetry

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// What a run queues waits for its own command's event, and is taken once.
func TestRenamedFlagsWaitForTheirCommand(t *testing.T) {
	t.Cleanup(func() { queuedRenamedFlags = nil })
	cmd, other := &cobra.Command{Use: "list"}, &cobra.Command{Use: "other"}

	RecordRenamedFlags(cmd, []string{"--workspace-id", "--deployment-id"}, []string{"--workspace", "--deployment"})
	assert.Nil(t, takeRenamedFlags(other), "another command's event takes nothing, and empties the queue")
	assert.Nil(t, queuedRenamedFlags)

	RecordRenamedFlags(cmd, []string{"--workspace-id"}, []string{"--workspace"})
	q := takeRenamedFlags(cmd)
	if assert.NotNil(t, q) {
		assert.Equal(t, []string{"--workspace-id"}, q.renamed)
		assert.Equal(t, []string{"--workspace"}, q.current)
	}
	assert.Nil(t, takeRenamedFlags(cmd))

	RecordRenamedFlags(cmd, nil, nil)
	assert.Nil(t, queuedRenamedFlags, "a run that gave no 1.x spelling queues nothing")
}
