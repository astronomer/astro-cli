package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newAICommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ai [message]",
		Short: "AI-related commands for Astro",
		Long:  `AI-powered capabilities for the Astro CLI.`,
		Args:  cobra.MinimumNArgs(1),
		Run:   printRobotASCII,
	}

	return cmd
}

func printRobotASCII(cmd *cobra.Command, args []string) {
	message := strings.Join(args, " ")
	messageLen := len(message)

	// Create the speech bubble
	topBorder := " " + strings.Repeat("_", messageLen+2)
	messageLine := "< " + message + " >"
	bottomBorder := " " + strings.Repeat("-", messageLen+2)

	robot := `
  ┌─┐
  │o│
 ╱───╲
 │╳─╳│
 ╰───╯
  │ │
  └ ┘
`

	fmt.Println(topBorder)
	fmt.Println(messageLine)
	fmt.Println(bottomBorder)
	fmt.Println("    \\")
	fmt.Println("     \\")
	fmt.Println(robot)
}
