package apc

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	giturls "github.com/whilp/git-urls"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/internal/platform/apc/workspace"
)

var (
	ErrInvalidDAGDeploymentType = errors.New("please specify the correct Dag deployment type, one of the following: image, volume, git_sync(if enabled in platform config), dag_deploy(if enabled in platform config)")
	errNFSLocationNotFound      = errors.New("please specify the nfs location via --nfs-location flag")
	errGitRepoNotFound          = errors.New("please specify a valid git repository URL via --git-repository-url")
	errInvalidExecutorType      = errors.New("please specify correct executor, one of: local, celery, kubernetes, k8s, or Astro's spellings LocalExecutor, CeleryExecutor, KubernetesExecutor")
	errAstroExecutorOnAPC       = errors.New("AstroExecutor is an Astro executor, and Astro Private Cloud has no equivalent: specify one of local, celery, kubernetes (or LocalExecutor, CeleryExecutor, KubernetesExecutor)")
	errNoWorkspaceFound         = errors.New("no valid workspace source found")
	errInvalidDeploymentMode    = errors.New("please specify a valid deployment mode, one of: helm, operator")
)

var validGitScheme = map[string]struct{}{
	"git":   {},
	"ssh":   {},
	"http":  {},
	"https": {},
}

// coalesceWorkspace is the workspace a command acts in: --workspace, else
// the context's. The flag is read first, so a context with no workspace, or
// none at all, does not fail a command that names one.
func coalesceWorkspace() (string, error) {
	if workspaceID != "" {
		return workspaceID, nil
	}

	wsCfg, err := workspace.GetCurrentWorkspace()
	if err != nil {
		return "", fmt.Errorf("failed to get current workspace: %w", err)
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

func validateDeploymentModeArg(mode string) error {
	switch mode {
	case "", houston.HelmDeploymentMode, houston.OperatorDeploymentMode:
		return nil
	default:
		return errInvalidDeploymentMode
	}
}

// validateExecutorArg is the executor --executor names, in APC's spelling
// (celery) or Astro's (CeleryExecutor), either in any case.
func validateExecutorArg(executor string) (string, error) {
	var executorType string
	switch strings.ToLower(executor) {
	case localExecutorArg, "localexecutor":
		executorType = houston.LocalExecutorType
	case celeryExecutorArg, "celeryexecutor":
		executorType = houston.CeleryExecutorType
	case kubernetesExecutorArg, k8sExecutorArg, "kubernetesexecutor":
		executorType = houston.KubernetesExecutorType
	case "astroexecutor":
		return executorType, errAstroExecutorOnAPC
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
