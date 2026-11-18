package telemetry

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// A command recorded twice in one run (its flag alias, then its own deprecated
// path) sends one event holding both.
func TestRenamedFlagsOfOneCommandAreOneEvent(t *testing.T) {
	t.Cleanup(func() { queuedRenamedFlags = nil })
	cmd := &cobra.Command{Use: "add"}
	RecordRenamedFlags(cmd, []string{"--org-token-name"}, []string{"--name"})
	RecordRenamedFlags(cmd, []string{"a b add"}, []string{"a add"})
	q := takeRenamedFlags(cmd)
	if assert.NotNil(t, q) {
		assert.Equal(t, []string{"--org-token-name", "a b add"}, q.renamed)
		assert.Equal(t, []string{"--name", "a add"}, q.current)
	}
}

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
