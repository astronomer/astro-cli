package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/astronomerio/astro-cli/config"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// RootCmd is the astro root command.
var (
	workspaceId string
	RootCmd     = &cobra.Command{
		Use:   "astro",
		Short: "Astronomer - CLI",
		Long:  "astro is a command line interface for working with the Astronomer Platform.",
	}
)

func init() {
	cobra.OnInitialize(config.InitConfig)
}

func isValidUpdateAttr(arg string, valids []string) bool {
	for _, e := range valids {
		if e == arg {
			return true
		}
	}
	return false
}

func updateArgValidator(args, validArgs []string) error {
	if len(args) < 1 {
		return errors.New("requires at least one arg")
	}

	for _, kv := range args[1:] {
		split := strings.Split(kv, "=")
		if len(split) == 1 {
			return fmt.Errorf("Failed to parse key value pair (%s)", kv)
		}

		k := split[0]
		if !isValidUpdateAttr(k, validArgs) {
			return fmt.Errorf("invalid update arg key specified (%s)", k)
		}
	}

	return nil
}

func workspaceValidator() string {
	wsFlag := workspaceId
	wsCfg := config.CFG.ProjectWorkspace.GetString()

	if len(wsFlag) != 0 {
		return wsFlag
	}

	if len(wsCfg) != 0 {
		return wsCfg
	}

	fmt.Println("Default workspace id not set, set default workspace id or pass a workspace in via the --workspace-id flag")
	os.Exit(1)

	return ""
}
