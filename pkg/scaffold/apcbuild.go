package scaffold

import (
	"regexp"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// Astro Private Cloud's `astro deploy` builds only the 1.x layout: it requires
// .astro/config.yaml, which a conversion keeps, and builds the Dockerfile as it
// stands, whose Astro Runtime base installs requirements.txt and packages.txt
// through its ONBUILD steps. A conversion that retired a pin-only Dockerfile
// and the two lists it migrated left an APC project that passed the deploy's
// project check and then failed its build.
//
// So where the project deploys is a fact the conversion needs, and it is
// decided once (resolveDeployTarget) and handed to every decision that turns
// on it: what planRetirements may retire, whether .dockerignore is written for
// that build's context, whether a kept Dockerfile's FROM must agree with the
// pin (refuseKeptDockerfileOfAnotherAirflow), and whether the saved deploy
// target becomes an Astro link (deployLink). Deciding them separately is how a
// run once kept the APC build and wrote an Astro link in the same manifest.

// deployTarget is where the converted project deploys, as far as the run can
// tell, and how it knows.
type deployTarget struct {
	// apc is true for Astro Private Cloud; false is Astro, which builds a
	// manifest project without any of the 1.x files.
	apc bool
	// why says what decided apc, for the note: it finishes "kept for Astro
	// Private Cloud (...)".
	why string
}

// resolveDeployTarget decides where the project deploys: from the project when
// its files say, and from the caller's current context (Options.DeploysToAPC)
// when they do not.
//
// The project says only one thing on its own. .astro/config.yaml's
// project.deployment is the target `astro deploy --save` remembered, and the
// 0.x CLI's Software deploy saved the Deployment's release name there, which
// Houston generates in one shape (releaseNameRe) that no Astro Deployment id
// has. A cuid says nothing either way, because Houston's ids are cuids too and
// 1.x's APC deploy saves those, so a saved cuid pair is left to the context,
// and deployLink follows the answer rather than supplying it. Any other value
// is not read as a signal: it is no Astro id, but nothing says it is APC's
// either.
func resolveDeployTarget(deploysToAPC bool, from1x *project1x) deployTarget {
	if releaseNameRe.MatchString(from1x.deployment) {
		return deployTarget{apc: true, why: config1xRelPath + " saves " + from1x.deployment +
			" as its deploy target, an Astro Private Cloud release name"}
	}
	if deploysToAPC {
		return deployTarget{apc: true, why: "the current context"}
	}
	return deployTarget{}
}

// releaseNameRe is the release name Houston gives a Software Deployment: an
// adjective, a noun and four digits, as in "celestial-gravity-1234".
var releaseNameRe = regexp.MustCompile(`^[a-z]+-[a-z]+-\d{4}$`)

// hasDockerfile reports a Dockerfile in the project, whatever it says.
func (from1x *project1x) hasDockerfile() bool {
	return slices.Contains(from1x.present, fileDockerfile)
}

// onbuildKind is what a Dockerfile's base does with the 1.x lists at build time.
type onbuildKind int

const (
	// onbuildUnknown is a base this cannot read, or one that is not an Astro
	// Runtime image.
	onbuildUnknown onbuildKind = iota
	// onbuildInstalls is an Astro Runtime image, whose ONBUILD steps install
	// requirements.txt and packages.txt from the build context.
	onbuildInstalls
	// onbuildNone is a -base Astro Runtime image, built without those steps.
	onbuildNone
)

// runtimeOnbuild is what an Astro Runtime tag's flavor does with the lists. The
// flavor is the tag's last suffix, after any -python-X.Y, which is where
// airflowrt.RuntimePythonRe reads it too.
func runtimeOnbuild(tag string) onbuildKind {
	if strings.HasSuffix(tag, "-base") {
		return onbuildNone
	}
	return onbuildInstalls
}

// apcBuildNotes is the note saying why kept, the files planRetirements kept for
// APC alone, survive, and what that asks of the user: one note, or none when it
// kept nothing, so a caller appends it unconditionally.
//
// Each clause is about what it names: the Dockerfile is said to be kept for APC
// only when it is in kept (one kept for another reason, a version the pin did
// not take or an instruction past its FROM, has its own note), the lists are
// said to be installed only by a base that installs them, and pyproject.toml is
// said to carry the same only for what was kept here, which is exactly what
// would otherwise have retired because the manifest carries all of it.
func apcBuildNotes(target deployTarget, from1x *project1x, kept []string) []string {
	if !target.apc || len(kept) == 0 {
		return nil
	}
	// In the order a manifest states them, whatever order kept came in.
	var lists, same []string
	keptDockerfile := slices.Contains(kept, fileDockerfile)
	if keptDockerfile {
		// Kept here only when spent: its tag is the pin that won.
		same = append(same, "Airflow version")
	}
	for _, f := range []struct{ name, carried string }{{fileRequirements, "dependencies"}, {filePackages, "OS packages"}} {
		if slices.Contains(kept, f.name) {
			lists = append(lists, f.name)
			same = append(same, f.carried)
		}
	}

	var b strings.Builder
	b.WriteString(joinNames(kept) + ": kept for Astro Private Cloud (" + target.why + "), whose `astro deploy` " +
		"builds the project from its Dockerfile as it stands")
	if !keptDockerfile {
		b.WriteString(", which this run keeps as well")
	}
	if len(lists) > 0 {
		installs := joinNames(lists)
		switch from1x.onbuild {
		case onbuildInstalls:
			b.WriteString("; its runtime base image installs " + installs + " during that build")
		case onbuildNone:
			b.WriteString("; its -base runtime image runs no ONBUILD steps, so that build installs " + installs +
				" only if the Dockerfile does")
		case onbuildUnknown:
			b.WriteString("; that build installs " + installs + " only if the Dockerfile or its base image does")
		}
	}
	carried := same[len(same)-1]
	if len(same) > 1 {
		carried = strings.Join(same[:len(same)-1], ", ") + " and " + carried
	}
	b.WriteString(". " + manifest.Marker + " carries the same " + carried + " for `astro local` and Astro, " +
		"so change both together while the project deploys to Astro Private Cloud, and delete " + pronoun(len(kept)) +
		" if it deploys to Astro instead")
	return []string{b.String()}
}
