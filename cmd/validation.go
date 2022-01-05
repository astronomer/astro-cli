package cmd

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/workspace"

	"github.com/pkg/errors"
	giturls "github.com/whilp/git-urls"
)

var (
	volumeDeploymentType  = "volume"
	imageDeploymentType   = "image"
	gitSyncDeploymentType = "git_sync"

	validGitScheme = map[string]struct{}{
		"git":   {},
		"ssh":   {},
		"http":  {},
		"https": {},
	}
)

type ErrParsingKV struct {
	kv string
}

func (e ErrParsingKV) Error() string {
	return fmt.Sprintf("failed to parse key value pair (%s)", e.kv)
}

type ErrInvalidArg struct {
	key string
}

func (e ErrInvalidArg) Error() string {
	return fmt.Sprintf("invalid update arg key specified (%s)", e.key)
}

func argsToMap(args []string) (map[string]string, error) {
	argsMap := make(map[string]string)
	for _, kv := range args {
		split := strings.Split(kv, "=")
		if len(split) == 1 {
			return nil, ErrParsingKV{kv: kv}
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
			return ErrParsingKV{kv: kv}
		}
		k := split[0]
		if !isValidUpdateAttr(k, validArgs) {
			return ErrInvalidArg{key: k}
		}
	}

	return nil
}

func coalesceWorkspace() (string, error) {
	wsFlag := workspaceID
	wsCfg, err := workspace.GetCurrentWorkspace()
	if err != nil {
		return "", errors.Wrap(err, "failed to get current workspace")
	}

	if wsFlag != "" {
		return wsFlag, nil
	}

	if wsCfg != "" {
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

func validateDagDeploymentArgs(dagDeploymentType, nfsLocation, gitRepoURL string, acceptEmptyArgs bool) error {
	if dagDeploymentType != imageDeploymentType && dagDeploymentType != volumeDeploymentType && dagDeploymentType != gitSyncDeploymentType && dagDeploymentType != "" {
		return errors.New("please specify the correct DAG deployment type, one of the following: image, volume, git_sync")
	}
	if dagDeploymentType == volumeDeploymentType && nfsLocation == "" {
		return errors.New("please specify the nfs location via --nfs-location flag")
	}
	if dagDeploymentType == gitSyncDeploymentType && !validURL(gitRepoURL, acceptEmptyArgs) {
		return errors.New("please specify a valid git repository URL via --git-repository-url")
	}
	return nil
}

func validateExecutorArg(executor string) (string, error) {
	var executorType string
	switch executor {
	case "local":
		executorType = "LocalExecutor"
	case "celery":
		executorType = "CeleryExecutor"
	case "kubernetes", "k8s":
		executorType = "KubernetesExecutor"
	default:
		return executorType, errors.New("please specify correct executor, one of: local, celery, kubernetes, k8s")
	}
	return executorType, nil
}

// validURL will validate whether the URL's scheme is a known Git transport
func validURL(gitURL string, acceptEmptyURL bool) bool {
	if !acceptEmptyURL && gitURL == "" {
		return false
	} else if acceptEmptyURL && gitURL == "" {
		return true
	}
	u, err := giturls.Parse(gitURL)
	if err != nil {
		return false
	}
	// Parsing http & https URLs via more stricter ParseRequestURI
	if strings.HasPrefix(gitURL, "http") || strings.HasPrefix(gitURL, "https") {
		if _, err := url.ParseRequestURI(gitURL); err != nil {
			return false
		}
	}
	_, ok := validGitScheme[u.Scheme]
	return ok
}
