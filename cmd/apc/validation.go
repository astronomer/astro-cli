package apc

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	giturls "github.com/whilp/git-urls"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/internal/platform/apc/workspace"
)

var (
	ErrInvalidDAGDeploymentType = errors.New("invalid Dag deployment type: use image, volume, git_sync (if enabled in the platform config), or dag_deploy (if enabled in the platform config)")
	errNFSLocationNotFound      = cliout.Usage(errors.New("set the NFS location with --nfs-location"))
	errGitRepoNotFound          = cliout.Usage(errors.New("set a valid git repository URL with --git-repository-url"))
	errInvalidExecutorType      = errors.New("invalid executor: use local, celery, kubernetes, k8s, LocalExecutor, CeleryExecutor, or KubernetesExecutor")
	errAstroExecutorOnAPC       = errors.New("AstroExecutor is not available on Astro Private Cloud; use local, celery, kubernetes, LocalExecutor, CeleryExecutor, or KubernetesExecutor")
	errNoWorkspaceFound         = errors.New("no valid Workspace source found")
	errInvalidDeploymentMode    = errors.New("invalid Deployment mode: use helm or operator")
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
		return "", fmt.Errorf("could not get the current Workspace: %w", err)
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
	return fmt.Errorf("use one of: %s", strings.Join(validRoles, ", "))
}

func validateDeploymentRole(role string) error {
	validRoles := []string{houston.DeploymentAdminRole, houston.DeploymentEditorRole, houston.DeploymentViewerRole}

	for _, validRole := range validRoles {
		if role == validRole {
			return nil
		}
	}
	return fmt.Errorf("use one of: %s", strings.Join(validRoles, ", "))
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
