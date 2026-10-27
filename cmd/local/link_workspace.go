package local

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// `astro link workspace` sets the workspace link, [tool.astro] workspace,
// domain and organization, through scaffold.SetWorkspaceLink, the writer Astro
// Desktop links a workspace with. The two tools then write the same three keys
// by the same rules, and a project linked in either reads the same workspace
// (docs/workspace-link.md).

// The flag names `astro link workspace` registers beside --unset.
const (
	flagLinkDomain       = "domain"
	flagLinkOrganization = "organization"
)

// The status values of a workspaceLinkResult. unchanged is linkStatusUnchanged.
const (
	linkStatusLinked   = "linked"
	linkStatusUnlinked = "unlinked"
)

// workspaceLinkResult is what `astro link workspace` reports, the same value in
// text and json. Workspace, Domain and Organization are the keys as the
// manifest holds them after the write, read back rather than predicted, so an
// organization the writer kept is reported too. Pinned names the Deployment
// links the old workspace was written onto, so none of them moved.
type workspaceLinkResult struct {
	Workspace    string   `json:"workspace,omitempty"`
	Domain       string   `json:"domain,omitempty"`
	Organization string   `json:"organization,omitempty"`
	Status       string   `json:"status"`
	Manifest     string   `json:"manifest"`
	Pinned       []string `json:"pinned,omitempty"`
}

// workspaceLinkInput holds `astro link workspace`'s flags.
type workspaceLinkInput struct {
	domain, organization string
	unset                bool
}

func newLinkWorkspaceCmd(c *cli) *cobra.Command {
	in := &workspaceLinkInput{}
	cmd := &cobra.Command{
		Use:   "workspace [WORKSPACE_ID]",
		Short: "Link an Astro workspace to this project",
		Long: "Link an Astro workspace to this project. Its Environment Manager connections, Airflow " +
			"Variables and environment variables then reach local Airflow, below every local source, " +
			"and a Deployment link with no workspace of its own is looked up in it. In a terminal, with " +
			"no WORKSPACE_ID, it asks which of your workspaces to link.\n\n" +
			"The domain is --domain, else your current Astro login's, else the one the project already " +
			"names. The organization is --organization, else your current login's when the domain is " +
			"that login's.\n\n" +
			"Switching or unlinking first writes the old workspace onto each Deployment link that " +
			"inherited it, so no link moves.",
		Example: "  astro link workspace                          # pick one of your workspaces\n" +
			"  astro link workspace clx123abc\n" +
			"  astro link workspace clx123abc --domain astronomer-dev.io\n" +
			"  astro link workspace --unset",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			given := cmd.Flags().Changed
			if in.unset {
				switch {
				case len(args) > 0:
					return errors.New("--unset takes no workspace: it unlinks the one linked")
				case given(flagLinkDomain) || given(flagLinkOrganization):
					return fmt.Errorf("--unset removes the domain and organization with the workspace: drop --%s and --%s", flagLinkDomain, flagLinkOrganization)
				}
				return c.runLinkWorkspace(in, "")
			}
			if len(args) == 1 {
				return c.runLinkWorkspace(in, args[0])
			}
			if !c.mayPrompt() || c.d.PickWorkspace == nil {
				return input.Required(errors.New("name the workspace to link: astro link workspace WORKSPACE_ID, or pass --unset to unlink it"))
			}
			ws, err := c.d.PickWorkspace()
			if err != nil {
				return err
			}
			return c.runLinkWorkspace(in, ws)
		},
	}
	f := cmd.Flags()
	f.StringVar(&in.domain, flagLinkDomain, "", "Astro domain the workspace is on, such as astronomer.io (default: your current login's)")
	f.StringVar(&in.organization, flagLinkOrganization, "", "Organization the workspace is in (default: your current login's, on its domain)")
	f.BoolVar(&in.unset, flagLinkUnset, false, "Unlink the workspace")
	return cmd
}

// runLinkWorkspace links workspace ws, or unlinks the project's when ws is
// empty, and reports the manifest as the write left it.
func (c *cli) runLinkWorkspace(in *workspaceLinkInput, ws string) error {
	r := c.renderer()
	dir, path, m, err := c.linkProject()
	if err != nil {
		return err
	}
	ws = strings.TrimSpace(ws)
	if ws == "" && !in.unset {
		return errors.New("the workspace ID is empty: name one, or pass --unset to unlink the workspace")
	}
	domain, organization := c.workspaceLinkCoordinates(in, m)
	var pinned []string
	changed, err := watchManifest(dir, func(wrap func(run func() error) error) error {
		var werr error
		pinned, werr = scaffold.SetWorkspaceLink(dir, wrap, ws, domain, organization)
		return werr
	})
	if err != nil {
		return err
	}
	after, err := manifest.Load(path)
	if err != nil {
		return err
	}
	res := workspaceLinkResult{
		Workspace:    after.Astro.Workspace,
		Domain:       after.Astro.Domain,
		Organization: after.Astro.Organization,
		Status:       linkStatusLinked,
		Manifest:     path,
		Pinned:       pinned,
	}
	switch {
	case !changed:
		res.Status = linkStatusUnchanged
	case ws == "":
		res.Status = linkStatusUnlinked
		// What was unlinked, since the keys are gone from the file.
		res.Workspace = m.Astro.Workspace
	}
	return r.Emit(res, func(w io.Writer) error {
		return writeWorkspaceLinkResult(w, res, m.Astro.Workspace)
	})
}

// workspaceLinkCoordinates is the domain and organization a link writes. The
// domain is --domain; else the current Astro login's, since that is the login
// a workspace is picked from and the one its id was most likely copied from;
// else the domain the project already reads its workspace on. The organization
// is --organization; else the current login's, but only on that login's domain,
// where it is the organization the workspace was listed in. Empty leaves the
// writer's rule: keep the one recorded for the workspace already linked.
func (c *cli) workspaceLinkCoordinates(in *workspaceLinkInput, m *manifest.Manifest) (domain, organization string) {
	current := ""
	if c.d.CurrentDomain != nil {
		current = manifest.NormalizeDomain(c.d.CurrentDomain())
	}
	domain = manifest.NormalizeDomain(in.domain)
	if domain == "" {
		domain = current
	}
	if domain == "" {
		domain = m.Astro.WorkspaceDomain()
	}
	organization = strings.TrimSpace(in.organization)
	if organization == "" && domain == current && c.d.CurrentOrganization != nil {
		organization = c.d.CurrentOrganization()
	}
	return domain, organization
}

// writeWorkspaceLinkResult is the text form of res. before is the workspace
// the project linked before the write, which the pinned note names.
func writeWorkspaceLinkResult(w io.Writer, res workspaceLinkResult, before string) error {
	var err error
	switch res.Status {
	case linkStatusUnlinked:
		_, err = fmt.Fprintf(w, "unlinked workspace %s from %s\n", res.Workspace, res.Manifest)
	case linkStatusUnchanged:
		if res.Workspace == "" {
			_, err = fmt.Fprintf(w, "no workspace is linked in %s\n", res.Manifest)
		} else {
			_, err = fmt.Fprintf(w, "workspace %s on %s is already linked in %s\n", res.Workspace, res.Domain, res.Manifest)
		}
	default:
		where := res.Domain
		if res.Organization != "" {
			where += ", organization " + res.Organization
		}
		_, err = fmt.Fprintf(w, "linked workspace %s (%s) in %s\n", res.Workspace, where, res.Manifest)
	}
	if err != nil || len(res.Pinned) == 0 {
		return err
	}
	_, err = fmt.Fprintf(w, "kept %s in workspace %s: %s inherited the project's workspace, so it was written onto each\n",
		strings.Join(res.Pinned, ", "), before, pluralThey(len(res.Pinned)))
	return err
}

// pluralThey is "it" for one link and "they" for more.
func pluralThey(n int) string {
	if n == 1 {
		return "it"
	}
	return "they"
}
