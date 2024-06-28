package software

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	giturls "github.com/whilp/git-urls"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/software/workspace"
)

var (
	ErrInvalidDAGDeploymentType = errors.New("please specify the correct DAG deployment type, one of the following: image, volume, git_sync(if enabled in platform config), dag_deploy(if enabled in platform config)")
	errNFSLocationNotFound      = errors.New("please specify the nfs location via --nfs-location flag")
	errGitRepoNotFound          = errors.New("please specify a valid git repository URL via --git-repository-url")
	errInvalidExecutorType      = errors.New("please specify correct executor, one of: local, celery, kubernetes, k8s")
	errNoWorkspaceFound         = errors.New("no valid workspace source found")
)

var validGitScheme = map[string]struct{}{
	"git":   {},
	"ssh":   {},
	"http":  {},
	"https": {},
}

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

func coalesceWorkspace() (string, error) {
	wsFlag := workspaceID
	wsCfg, err := workspace.GetCurrentWorkspace()
	if err != nil {
		return "", fmt.Errorf("failed to get current workspace: %w", err)
	}

	if wsFlag != "" {
		return wsFlag, nil
	}

	if wsCfg != "" {
		return wsCfg, nil
	}

	return "", errNoWorkspaceFound
}

func validateWorkspaceRole(role string) error {
	validRoles := []string{houston.WorkspaceAdminRole, houston.WorkspaceEditorRole, houston.WorkspaceViewerRole}

	for _, validRole := range validRoles {
		if role == validRole {
			return nil
		}
	}
	return fmt.Errorf("please use one of: %s", strings.Join(validRoles, ", "))
}

func validateDeploymentRole(role string) error {
	validRoles := []string{houston.DeploymentAdminRole, houston.DeploymentEditorRole, houston.DeploymentViewerRole}

	for _, validRole := range validRoles {
		if role == validRole {
			return nil
		}
	}
	return fmt.Errorf("please use one of: %s", strings.Join(validRoles, ", "))
}

func validateDagDeploymentArgs(dagDeploymentType, nfsLocation, gitRepoURL string, acceptEmptyArgs bool) error {
	if dagDeploymentType != houston.ImageDeploymentType &&
		dagDeploymentType != houston.VolumeDeploymentType &&
		dagDeploymentType != houston.GitSyncDeploymentType &&
		dagDeploymentType != houston.DagOnlyDeploymentType &&
		dagDeploymentType != "" {
		return ErrInvalidDAGDeploymentType
	}
	if dagDeploymentType == houston.VolumeDeploymentType && nfsLocation == "" {
		return errNFSLocationNotFound
	}
	if dagDeploymentType == houston.GitSyncDeploymentType && !validURL(gitRepoURL, acceptEmptyArgs) {
		return errGitRepoNotFound
	}
	return nil
}

func validateExecutorArg(executor string) (string, error) {
	var executorType string
	switch executor {
	case localExecutorArg:
		executorType = houston.LocalExecutorType
	case celeryExecutorArg:
		executorType = houston.CeleryExecutorType
	case kubernetesExecutorArg, k8sExecutorArg:
		executorType = houston.KubernetesExecutorType
	default:
		return executorType, errInvalidExecutorType
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
