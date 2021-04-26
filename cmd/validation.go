package cmd

import (
	"fmt"
	"strings"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/workspace"

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

func coalesceWorkspace() (string, error) {
	wsFlag := workspaceId
	wsCfg, err := workspace.GetCurrentWorkspace()
	if err != nil {
		return "", errors.Wrap(err, "failed to get current workspace")
	}

	if len(wsFlag) != 0 {
		return wsFlag, nil
	}

	if len(wsCfg) != 0 {
		return wsCfg, nil
	}

	return "", errors.New("no valid workspace source found")
}

func validateWorkspaceRole(role string) error {
	validRoles := []string{"WORKSPACE_ADMIN", "WORKSPACE_EDITOR", "WORKSPACE_VIEWER"}

	for _, validRole := range validRoles {
		if role == validRole {
			return nil
		}
	}
	return errors.Errorf("please use one of: %s", strings.Join(validRoles, ", "))
}

func validateDeploymentRole(role string) error {
	validRoles := []string{houston.DeploymentAdmin, houston.DeploymentEditor, houston.DeploymentViewer}

	for _, validRole := range validRoles {
		if role == validRole {
			return nil
		}
	}
	return errors.Errorf("please use one of: %s", strings.Join(validRoles, ", "))
}

func validateRole(role string) error {
	validRoles := []string{"admin", "editor", "viewer"}

	for _, validRole := range validRoles {
		if role == validRole {
			return nil
		}
	}
	return errors.Errorf("please use one of: %s", strings.Join(validRoles, ", "))
}

func validateDagDeploymentArgs(dagDeploymentType, nfsLocation string) error {
	if dagDeploymentType != "image" && dagDeploymentType != "volume" {
 		return errors.New("please specify the correct DAG deployment type, one of the following: image, volume")
	}
	if dagDeploymentType == "volume" && nfsLocation == "" {
 		return errors.New("please specify the nfs location via --nfs-location flag")
	}
	return nil
}
