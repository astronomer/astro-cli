package astro

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/astrosession"
)

// A command that fails looking something up in the Workspace it was pointed at
// should say where that Workspace came from: "workspace B (from --workspace)
// was not found" tells a person which of the flag, the project link and the
// token to change, and a bare id does not. The lookup is made deep in a
// platform package that is handed an id, so the id is worded here, once, at
// the command: workspaceTarget records the resolver's answer for the run
// (rememberTarget), and the RunE of every command in the tree
// (explainWorkspaceErrors) words the error it returns by it
// (astrosession.Target.Explain). The error's kind and code are unchanged: it
// unwraps to what the platform returned.

type targetBoxKey struct{}

// targetBox is where workspaceTarget records the Target a command run
// resolved last, for the command's RunE wrapper to read when the run fails.
type targetBox struct {
	t   astrosession.Target
	set bool
}

// rememberTarget records t as cmd's run's Workspace, if cmd runs under
// explainWorkspaceErrors. A command built by hand, as a test does, has no
// box and records nothing.
func rememberTarget(cmd *cobra.Command, t astrosession.Target) { //nolint:gocritic // a small value type
	if cmd == nil || cmd.Context() == nil {
		return
	}
	if box, ok := cmd.Context().Value(targetBoxKey{}).(*targetBox); ok {
		box.t, box.set = t, true
	}
}

// explainWorkspaceErrors wraps the RunE of cmd and every command under it, so
// an error a run returns about its Workspace names where the Workspace came
// from. A command with no RunE is a group, and is left as it is.
func explainWorkspaceErrors(cmd *cobra.Command) {
	if run := cmd.RunE; run != nil {
		cmd.RunE = func(c *cobra.Command, args []string) error {
			box := &targetBox{}
			parent := c.Context()
			if parent == nil {
				parent = context.Background()
			}
			c.SetContext(context.WithValue(parent, targetBoxKey{}, box))
			err := run(c, args)
			if err != nil && box.set {
				return box.t.Explain(err)
			}
			return err
		}
	}
	for _, sub := range cmd.Commands() {
		explainWorkspaceErrors(sub)
	}
}
