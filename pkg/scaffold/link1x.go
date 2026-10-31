package scaffold

import (
	"regexp"
	"slices"

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

// deployLinkAdvisory says what setDeployLink wrote, or "" when it wrote nothing:
// what was linked first, then, for a workspace taken from the context, what
// to run if it is the wrong one. One line, as every entry is: the text
// renderer indents each entry once, and a consumer reads each as a whole.
func (from1x *project1x) deployLinkAdvisory() string {
	l, ok := from1x.deployLink()
	if !ok {
		return ""
	}
	where := "workspace " + l.Workspace
	if l.Kind == manifest.KindAPC {
		where = "Astro Private Cloud " + l.Domain + ", " + where
	}
	out := config1xRelPath + ": linked Deployment " + l.Deployment + " as \"" + l.Name + "\" in " + manifest.Marker +
		" (" + where + ")"
	if from1x.workspaceFromContext {
		out += ". It saved no workspace, so this uses your current one. If the Deployment is in another workspace, " +
			"relink it with: astro link add " + l.Name + " --deployment " + l.Deployment + " --workspace <workspace-id> --replace"
	}
	return out
}

// instanceLinks are the links a 1.x .astro/config.yaml's instances list
// becomes when the caller can tell a Deployment's workspace
// (Options.InstanceWorkspace): each Astro entry with a plain name and a
// Deployment id whose workspace it tells, written as astro link add NAME
// --deployment ID --workspace WS writes it. Not under an Astro Private Cloud
// context, whose Deployments the Astro lookup does not know, and not an entry
// whose name another link already takes. The rest stay notes
// (unlinkedInstances). Decided once, so the platform is asked once an entry.
func (from1x *project1x) instanceLinks() []Link {
	if from1x.linked != nil {
		return *from1x.linked
	}
	var out []Link
	from1x.linked = &out
	if from1x.instanceWorkspace == nil || from1x.apcDomain != "" {
		return nil
	}
	taken := map[string]bool{manifest.ReservedLinkName: true, BundleLinkName: true}
	if l, ok := from1x.deployLink(); ok {
		taken[l.Name] = true
	}
	for _, inst := range from1x.instances {
		if inst.source != "" && inst.source != string(manifest.KindAstro) {
			continue
		}
		if !plainDeployID(inst.name) || !astroIDRe.MatchString(inst.deploymentID) || taken[inst.name] {
			continue
		}
		ws := from1x.instanceWorkspace(inst.deploymentID)
		if !astroIDRe.MatchString(ws) {
			continue
		}
		taken[inst.name] = true
		out = append(out, Link{Name: inst.name, Kind: manifest.KindAstro, Deployment: inst.deploymentID, Workspace: ws})
	}
	return out
}

// unlinkedInstances are the instances entries instanceLinks did not link.
func (from1x *project1x) unlinkedInstances() []instance1x {
	linked := from1x.instanceLinks()
	return slices.DeleteFunc(slices.Clone(from1x.instances), func(inst instance1x) bool {
		return slices.ContainsFunc(linked, func(l Link) bool { return l.Name == inst.name && l.Deployment == inst.deploymentID })
	})
}

// setInstanceLinks writes instanceLinks, each with SaveLink's own edit over
// the manifest as written so far, so the defaults come out as a run of astro
// link add would leave them.
func setInstanceLinks(ed tomledit.Editor, from1x *project1x) error {
	for _, l := range from1x.instanceLinks() {
		if err := l.validate(); err != nil {
			return err
		}
		data, err := ed.Bytes()
		if err != nil {
			return err
		}
		before, err := manifest.Parse(data)
		if err != nil {
			return err
		}
		if err := l.edit()(before, ed); err != nil {
			return err
		}
	}
	return nil
}

// instanceLinksAdvisory says which Deployments setInstanceLinks linked, or ""
// when it linked none.
func (from1x *project1x) instanceLinksAdvisory() string {
	links := from1x.instanceLinks()
	if len(links) == 0 {
		return ""
	}
	names := make([]string, len(links))
	for i, l := range links {
		names[i] = l.Name
	}
	noun := "Deployments"
	if len(links) == 1 {
		noun = "Deployment"
	}
	return manifest.Marker + ": linked the " + noun + " " + config1xRelPath + " lists, " + joinNames(names)
}
