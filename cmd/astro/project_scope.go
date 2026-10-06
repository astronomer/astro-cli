package astro

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/util"
)

// deploymentArgAnnotation marks a command whose first argument is a Deployment
// id, so a link name in that place resolves the way --deployment-id does.
const deploymentArgAnnotation = "astro.deployment-arg"

// projectWorkspaceID is the workspace a command inside a project falls back
// to before the context's. followProject sets it; coalesceWorkspace reads it.
var projectWorkspaceID string

// projectPick is what a project decided for a command the user left open.
type projectPick struct {
	// workspace is the default workspace, "" when the project picks none.
	workspace string
	// domain is the Astro host the command runs on, "" for the current one.
	domain string
	// link is the Deployment link the pick came from, "" for [tool.astro].
	link string
	// organization is [tool.astro] organization when the pick is the
	// project's workspace, "" when the manifest names none.
	organization string
}

// followProjectPreRun is the pre-run of `astro env` and `astro deployment`:
// inside a project they act on the project's workspace, on its host, and
// take a link name wherever they take a Deployment id. group is the command
// the hook is set on, so the hook can run the one above it, which cobra would
// otherwise skip.
func followProjectPreRun(group *cobra.Command) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		before, _ := config.GetCurrentContext() //nolint:errcheck // with no context there is nothing to compare, and the login check below reports it
		pick, err := followProject(cmd, args)
		if err != nil {
			return err
		}
		switched := pick.domain != "" && pick.domain != manifest.NormalizeDomain(before.Domain) && !credentialFromEnv()
		if switched {
			if _, err := astrosession.BearerFor(cmd.Context(), pick.domain); err != nil {
				return err
			}
			if err := os.Setenv("ASTRO_DOMAIN", pick.domain); err != nil {
				return err
			}
		}
		projectWorkspaceID = pick.workspace
		for p := group.Parent(); p != nil; p = p.Parent() {
			if p.PersistentPreRunE != nil {
				if err := p.PersistentPreRunE(cmd, args); err != nil {
					return err
				}
				break
			}
		}
		if note := projectNote(cmd.Context(), pick, before.Workspace, switched); note != "" {
			fmt.Fprintln(cmd.ErrOrStderr(), note)
		}
		return nil
	}
}

// followProject reads the project around the working directory, if any, and
// resolves the command's Deployment reference and default workspace from it.
// A link name given as --deployment-id, --deployment-name or a Deployment-id
// argument is replaced in place by the link's Deployment id. --deployment
// reaches here as one of those (see routeDeployment).
func followProject(cmd *cobra.Command, args []string) (projectPick, error) {
	projectWorkspaceID = ""
	m, err := projectManifest(config.WorkingPath)
	if err != nil || m == nil {
		return projectPick{}, err
	}

	var id, link string
	flags := cmd.Flags()
	switch {
	case flags.Changed("deployment-id"):
		f := flags.Lookup("deployment-id")
		if id, link, err = projectDeployment(m, f.Value.String()); err != nil {
			return projectPick{}, err
		}
		if err := f.Value.Set(id); err != nil {
			return projectPick{}, err
		}
	case flags.Changed("deployment-name"):
		f := flags.Lookup("deployment-name")
		l, ok := m.Astro.Deployments[f.Value.String()]
		if !ok || l.Kind() != manifest.KindAstro {
			break
		}
		link = f.Value.String()
		deploymentID = l.Deployment
		if err := f.Value.Set(""); err != nil {
			return projectPick{}, err
		}
	case cmd.Annotations[deploymentArgAnnotation] != "" && len(args) > 0:
		if id, link, err = projectDeployment(m, args[0]); err != nil {
			return projectPick{}, err
		}
		// The command's RunE is handed this same slice.
		args[0] = id
	case deploymentArg != "":
		// A Deployment id given as --deployment, standing in for the argument.
		if id, link, err = projectDeployment(m, deploymentArg); err != nil {
			return projectPick{}, err
		}
		deploymentID = id
	}

	pick := projectPick{link: link}
	if link != "" {
		pick.domain = m.Astro.LoginDomain()
	}
	if flags.Changed("workspace-id") || flags.Changed("all") {
		return pick, nil
	}
	if link != "" {
		pick.workspace = m.Astro.Deployments[link].Workspace
	} else if m.Astro.Workspace != "" {
		pick.workspace = m.Astro.Workspace
		pick.domain = m.Astro.WorkspaceDomain()
		pick.organization = m.Astro.Organization
	}
	return pick, nil
}

// projectManifest loads the manifest of the project holding dir, or nil
// outside one or in a project without [tool.astro].
func projectManifest(dir string) (*manifest.Manifest, error) {
	proj, err := project.Discover(dir)
	var notFound *project.NotFoundError
	if errors.As(err, &notFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m, err := manifest.Load(filepath.Join(proj.Dir, project.Marker))
	if errors.Is(err, manifest.ErrNoAstroSection) {
		return nil, nil
	}
	return m, err
}

// credentialFromEnv reports an API token or key in the environment. Setup
// uses one, for that process only, as the login of the domain it runs on and
// makes that domain current, so the command stays on the host the environment
// names rather than move the user's current context.
func credentialFromEnv() bool {
	return os.Getenv(astrosession.EnvAPIToken) != "" || os.Getenv("ASTRONOMER_KEY_ID") != ""
}

// projectDeployment reads ref as a Deployment link name, then as a Deployment
// id, and returns the id and the name of the link it belongs to, if any.
func projectDeployment(m *manifest.Manifest, ref string) (id, link string, err error) {
	if l, ok := m.Astro.Deployments[ref]; ok {
		if l.Kind() != manifest.KindAstro {
			return "", "", fmt.Errorf("link %s in pyproject.toml points at %s, not at an Astro Deployment", ref, l.Kind())
		}
		return l.Deployment, ref, nil
	}
	names := astroLinkNames(m)
	if util.IsCUID(ref) {
		for _, name := range names {
			if m.Astro.Deployments[name].Deployment == ref {
				return ref, name, nil
			}
		}
		return ref, "", nil
	}
	if len(names) == 0 {
		return "", "", fmt.Errorf("%q is not a Deployment id, and pyproject.toml has no Astro Deployment links", ref)
	}
	return "", "", fmt.Errorf("%q is not a Deployment id or a Deployment link in pyproject.toml. Links: %s", ref, strings.Join(names, ", "))
}

func astroLinkNames(m *manifest.Manifest) []string {
	var names []string
	for name := range m.Astro.Deployments {
		if m.Astro.Deployments[name].Kind() == manifest.KindAstro {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// projectNote is the line that says the project, not the context, chose where
// the command runs. It is empty when the project chose what the context would
// have.
func projectNote(ctx context.Context, pick projectPick, contextWorkspace string, switched bool) string {
	if !switched && (pick.workspace == "" || pick.workspace == contextWorkspace) {
		return ""
	}
	var what string
	switch {
	case pick.workspace != "" && switched:
		what = fmt.Sprintf("workspace %s on %s", workspaceLabel(ctx, pick.workspace, pick.organization), pick.domain)
	case pick.workspace != "":
		what = "workspace " + workspaceLabel(ctx, pick.workspace, pick.organization)
	default:
		what = pick.domain
	}
	if pick.link != "" {
		return fmt.Sprintf("using %s from pyproject.toml (link %s)", what, pick.link)
	}
	return fmt.Sprintf("using %s from pyproject.toml", what)
}

// workspaceLabel is the workspace's name, or its id when the name cannot be read.
// It is looked up under org, the organization the manifest names, else the
// current one.
func workspaceLabel(ctx context.Context, id, org string) string {
	c, err := config.GetCurrentContext()
	if err != nil {
		return id
	}
	resp, err := astroV1Client.GetWorkspaceWithResponse(ctx, (&manifest.Astro{Organization: org}).WorkspaceOrganization(c.Organization), id)
	if err != nil || resp.JSON200 == nil || resp.JSON200.Name == "" {
		return id
	}
	return resp.JSON200.Name
}
