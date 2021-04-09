package main

import (
	"fmt"
	"os"

	"github.com/astronomer/astro-cli/cmd"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/astrohub"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/spf13/afero"
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
	client := astrohub.NewAstrohubClient(httputil.NewHTTPClient())
	fs := afero.NewOsFs()
	config.InitConfig(fs)
	rootCmd := cmd.NewRootCmd(client, os.Stdout)
	doc.GenMarkdownTreeCustom(rootCmd, "./docs/", emptyStr, identity)
}
