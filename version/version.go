package version

import (
	"fmt"
)

var CurrVersion string

const (
	cliCurrentVersion = "Astro CLI Version: "
)

// PrintVersion outputs current cli version and git commit if exists
func PrintVersion() {
	version := CurrVersion
	fmt.Println(cliCurrentVersion + version)
}
