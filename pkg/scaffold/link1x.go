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
// A link needs a workspace to parse, and the conversion has no login to look
// one up with, so a saved target without one stays a note.
func (from1x *project1x) deployLink() (Link, bool) {
	if !astroIDRe.MatchString(from1x.deployment) || !astroIDRe.MatchString(from1x.workspace) {
		return Link{}, false
	}
	return Link{Name: Link1xName, Kind: manifest.KindAstro, Deployment: from1x.deployment, Workspace: from1x.workspace}, true
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
	return config1xRelPath + ": its saved deploy target is now the " + l.Name + " link in " + manifest.Marker +
		" ([tool.astro.deployments." + l.Name + "], deployment " + l.Deployment + " in workspace " + l.Workspace + ")"
}
