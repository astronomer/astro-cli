package astro

import (
	"strings"

	"github.com/spf13/cobra"
)

// The sections deploy's help splits its flags into, so it is clear which flags
// apply to which kind of deploy. Each value is the section's title: the root's
// help (cmd/help.go) lists a flag annotated "Image" under "Image Flags:".
const (
	deployGroupImage  = "Image"
	deployGroupDAG    = "Dag"
	deployGroupNonDAG = "Non-Dag Bundle"
)

// The annotation keys cmd/help.go reads a flag's section and the sections'
// order from. They are repeated rather than imported because cmd imports this
// package.
const (
	flagGroupAnnotation      = "group"
	flagGroupOrderAnnotation = "flag-groups"
)

// annotateDeployFlag puts a flag in one of deploy's help sections.
func annotateDeployFlag(cmd *cobra.Command, name, group string) {
	cmd.Flags().SetAnnotation(name, flagGroupAnnotation, []string{group}) //nolint:errcheck // error deliberately ignored in this shell code
}

// orderDeployFlagGroups lists deploy's sections in the order a deploy narrows:
// what image ships, which DAGs, and what rides beside.
func orderDeployFlagGroups(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[flagGroupOrderAnnotation] = strings.Join([]string{
		deployGroupImage, deployGroupDAG, deployGroupNonDAG,
	}, ",")
}
