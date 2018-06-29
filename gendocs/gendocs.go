package main

import (
	"fmt"

	"github.com/astronomerio/astro-cli/cmd"
	"github.com/spf13/cobra/doc"
)

// you can generate markdown docs by running
//
//   $ go run gendocs.go
//
// this also requires doc sub-package of cobra
// which is not kept in this repo
// you can acquire it by running
//
//   $ gvt restore

func main() {
	identity := func(s string) string {
		return fmt.Sprintf(`{{< relref "docs/%s" >}}`, s)
	}
	emptyStr := func(s string) string { return "" }
	doc.GenMarkdownTreeCustom(cmd.RootCmd, "./docs/", emptyStr, identity)
}
