package scaffold

import (
	"regexp"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// Link1xName is the name the conversion gives the deployment link it writes
// for a 1.x project's saved deploy target.
const Link1xName = "default"

// astroIDRe is the shape of an Astro Deployment or workspace id: a CUID, c and
// 24 lowercase alphanumerics (pkg/util.IsCUID, which this module cannot import).
var astroIDRe = regexp.MustCompile(`^c[a-z0-9]{24}$`)

// deployLink is the link a 1.x project's saved deploy target becomes, and
// whether it can become one.
//
// It can when .astro/config.yaml names both an Astro Deployment id and the
// workspace it lives in. The key also holds a Software release name, which
// cmd/apc/deploy.go saves there, and the id shape is what tells the two apart.
// A link needs a workspace to parse. Under Astro, a saved target without one
// stays a note: the conversion reads no Astro login to look one up.
//
// Under an Astro Private Cloud context (Options.APCDomain) the target is a
// Houston Deployment in a Houston workspace, also cuids, and becomes an apc
// link on that context's domain. A saved id without a workspace takes the
// context's current one (fillAPCWorkspace), asked for here, when the link is
// decided, so a run refused before that never asks.
func (from1x *project1x) deployLink() (Link, bool) {
	from1x.fillAPCWorkspace()
	if !astroIDRe.MatchString(from1x.deployment) || !astroIDRe.MatchString(from1x.workspace) {
		return Link{}, false
	}
	if from1x.apcDomain != "" {
		return Link{
			Name: Link1xName, Kind: manifest.KindAPC, Domain: from1x.apcDomain,
			Deployment: from1x.deployment, Workspace: from1x.workspace,
		}, true
	}
	return Link{Name: Link1xName, Kind: manifest.KindAstro, Deployment: from1x.deployment, Workspace: from1x.workspace}, true
}

// fillAPCWorkspace takes the workspace of a saved APC deploy target that
// .astro/config.yaml did not save from the APC context's current workspace
// (Options.APCWorkspace), which is where Astro CLI 1.x's deploy looked the
// saved id up. It asks at most once, and only when that decides something:
// under an APC context, for a saved Deployment id with no workspace beside
// it. A workspace that is not an id keeps the note.
func (from1x *project1x) fillAPCWorkspace() {
	current := from1x.apcWorkspace
	if current == nil || from1x.apcDomain == "" || from1x.workspace != "" || !astroIDRe.MatchString(from1x.deployment) {
		return
	}
	from1x.apcWorkspace = nil
	if ws := current(); astroIDRe.MatchString(ws) {
		from1x.workspace, from1x.workspaceFromContext = ws, true
	}
}

// setDeployLink writes the saved deploy target as a link marked default, with
// SaveLink's own edit, so it reads as `astro link add` would have written it.
//
// The edit is handed an empty manifest as the one it starts from, which is
// exactly true: both arms only reach here for a file with no [tool.astro] of
// its own, so there is no workspace, target or link for the new one to inherit
// or keep.
func setDeployLink(ed tomledit.Editor, from1x *project1x) error {
	l, ok := from1x.deployLink()
	if !ok {
		return nil
	}
	if err := l.validate(); err != nil {
		return err
	}
	if err := l.edit()(&manifest.Manifest{}, ed); err != nil {
		return err
	}
	return ed.Set(append(linkKey(l.Name), "default"), true)
}

// deployLinkAdvisory says what setDeployLink wrote, or "" when it wrote nothing.
func (from1x *project1x) deployLinkAdvisory() string {
	l, ok := from1x.deployLink()
	if !ok {
		return ""
	}
	where := ""
	if l.Kind == manifest.KindAPC {
		where = " on Astro Private Cloud " + l.Domain
	}
	out := config1xRelPath + ": its saved deploy target is now the " + l.Name + " link in " + manifest.Marker +
		" ([tool.astro.deployments." + l.Name + "], deployment " + l.Deployment + " in workspace " + l.Workspace + where + ")"
	if from1x.workspaceFromContext {
		out += ". The workspace is this context's current one, where Astro CLI 1.x looked the Deployment up, since " +
			config1xRelPath + " saved none; if the Deployment is not in it, astro deploy says so, and astro link add " +
			l.Name + " --deployment " + l.Deployment + " --workspace <id> --replace relinks it"
	}
	return out
}
