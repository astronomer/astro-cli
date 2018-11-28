package main

import (
	"os"

	"github.com/astronomer/astro-cli/cmd"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		// if cmd.Debug {
		// 	fmt.Printf("%+v", err)
		// }

		os.Exit(1)
	}
}
