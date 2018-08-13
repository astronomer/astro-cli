package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/astronomerio/astro-cli/workspace"

	"github.com/pkg/errors"
)

func argsToMap(args []string) (map[string]string, error) {
	argsMap := make(map[string]string)
	for _, kv := range args {
		split := strings.Split(kv, "=")
		if len(split) == 1 {
			return nil, fmt.Errorf("Failed to parse key value pair (%s)", kv)
		}

		argsMap[split[0]] = split[1]
	}
	return argsMap, nil
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
	wsCfg, err := workspace.GetCurrentWorkspace()
	if err != nil {
		return ""
	}

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
