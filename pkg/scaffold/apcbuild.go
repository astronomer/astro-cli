package scaffold

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// Astro Private Cloud's `astro deploy` builds only the 1.x layout: it requires
// .astro/config.yaml, which a conversion keeps, and builds the Dockerfile as it
// stands, whose Astro Runtime base installs requirements.txt and packages.txt
// through its ONBUILD steps. A conversion that retired a pin-only Dockerfile
// and the two lists it migrated left an APC project that passed the deploy's
// project check and then failed its build.
//
// So where the project deploys is a fact the conversion needs, and it is
// decided once, by the caller (resolveDeployTarget), and handed to every
// decision that turns on it: what planRetirements may retire, whether
// .dockerignore is written for that build's context, whether a kept
// Dockerfile's FROM must agree with the pin
// (refuseKeptDockerfileOfAnotherAirflow), and whether the saved deploy target
// becomes an Astro link (deployLink). Deciding them separately is how a run
// once kept the APC build and wrote an Astro link in the same manifest.

// DeployTargetBasis says what decided Options.DeploysToAPC and how to decide
// it the other way, so each message that turns on the decision can say both.
// The zero value says neither, and the messages read without them.
type DeployTargetBasis struct {
	// Why finishes "because ...": "the current context is Astro Private Cloud
	// (astro.example.com)", or "of --deploy-target apc".
	Why string
	// Instead is how to make a conversion target the other platform, as an
	// instruction: "pass --deploy-target astro". A message supplies the
	// condition itself ("to convert for Astro instead, ...").
	Instead string
}

// deployTarget is where the converted project deploys, what decided it, and,
// for APC, what the Dockerfile it builds says about its Airflow.
type deployTarget struct {
	// apc is true for Astro Private Cloud; false is Astro, which builds a
	// manifest project without any of the 1.x files.
	apc bool
	// basis is the caller's account of the decision.
	basis DeployTargetBasis
	// build is the FROM of a Dockerfile kept undeclared for APC's build; zero
	// when there is none.
	build apcBuild
	// undo is what to put back before converting again, once this run's
	// changes are planned (undoAdvice): a rerun is refused once the manifest
	// carries [tool.astro]. Empty before then, when a refusal has written
	// nothing and converting again needs nothing undone.
	undo string
}

// resolveDeployTarget takes where the project deploys from the caller
// (Options.DeploysToAPC), whether a flag, a choice in an app or the current
// context decided it there.
//
// The project's files do not decide it. .astro/config.yaml's
// project.deployment is the target `astro deploy --save` remembered, and no
// shape of it belongs to one platform: Houston's ids are cuids as Astro's are,
// an Astro Deployment's namespace has the adjective-noun-NNNN shape of a
// Software release name, and Houston takes a custom release name as well. So
// deployLink follows the answer rather than supplying it.
//
// For APC, the Dockerfile kept undeclared for its build is read here too,
// since its FROM is what the pin must agree with (apcBuild).
func resolveDeployTarget(dir string, opts *Options, from1x *project1x) deployTarget {
	t := deployTarget{apc: opts.DeploysToAPC, basis: opts.DeployTargetBasis}
	if buildsDockerfile(from1x, t.apc) && !declaresDockerfile(from1x) {
		t.build = apcBuildOf(dir, opts.RuntimeCatalog)
	}
	return t
}

// decided is the sentence saying which platform this run converted for, why,
// and how to choose the other: "" when the caller gave neither reason nor way.
func (t *deployTarget) decided() string {
	if t.basis.Why == "" && t.basis.Instead == "" {
		return ""
	}
	this, other := "Astro", "Astro Private Cloud"
	if t.apc {
		this, other = other, this
	}
	s := "This run converted the project for " + this
	if t.basis.Why != "" {
		s += " because " + t.basis.Why
	}
	if t.basis.Instead != "" {
		s += "; to convert for " + other + " instead, "
		if t.undo != "" {
			s += t.undo + ", then convert again and "
		}
		s += t.basis.Instead
	}
	return s
}

// adviseUndo sets undo once the run's changes are planned, since the notes'
// advice on converting for the other platform is read after the run, which a
// rerun then refuses. The saved deploy target's note, which leftovers wrote
// before the retirements were known, is the one note already carrying that
// advice, so it is rewritten in place; notes written later read undo as is.
func (t *deployTarget) adviseUndo(notes, removed []string, adopted bool) {
	before := t.decided()
	if before == "" {
		return
	}
	t.undo = undoAdvice(removed, adopted)
	for i, n := range notes {
		if strings.HasSuffix(n, before) {
			notes[i] = strings.TrimSuffix(n, before) + t.decided()
		}
	}
}

// undoAdvice is how to put back what a conversion changed that stops it
// running again: pyproject.toml, which it created or added [tool.astro] to,
// and the files it removes. The rest it writes (.gitignore lines, AGENTS.md,
// the dags folder) a rerun skips or leaves as it is.
func undoAdvice(removed []string, adopted bool) string {
	restore := slices.Sorted(slices.Values(removed))
	if adopted {
		restore = append(restore, manifest.Marker)
	}
	var said, cmds []string
	if len(restore) > 0 {
		said = append(said, "restore "+joinNames(restore)+" from version control")
		cmds = append(cmds, "git checkout -- "+strings.Join(restore, " "))
	}
	if !adopted {
		said = append(said, "delete the "+manifest.Marker+" this run created")
		cmds = append(cmds, "rm "+manifest.Marker)
	}
	return "first " + strings.Join(said, " and ") + " (`" + strings.Join(cmds, " && ") + "`)"
}

// hasDockerfile reports a Dockerfile with something in it to build. It is the
// one fact declaresDockerfile and buildsDockerfile share: an empty file
// builds nothing on any platform.
func (from1x *project1x) hasDockerfile() bool {
	return len(from1x.dockerfileBody) > 0
}

// buildsDockerfile reports that the project's Dockerfile will be built: it is
// declared as the build (declaresDockerfile), or the project deploys to APC
// (apc, see deployTarget), whose `astro deploy` builds the Dockerfile as it
// stands whatever the manifest says. It is what keeps the lists the build's
// base installs, what writes a .dockerignore for its context, and what holds
// the pin to its FROM.
func buildsDockerfile(from1x *project1x, apc bool) bool {
	return declaresDockerfile(from1x) || apc && from1x.hasDockerfile()
}

// apcBuild is what the FROM of a Dockerfile kept undeclared for APC says about
// the Airflow that deploys.
type apcBuild struct {
	// base is the FROM read.
	base airflowrt.DeclaredBase
	// tag is base's tag as manifest.ParseRuntimeTag reads it, when parsed
	// says it could. An Airflow 2 tag names a runtime version, not an Airflow
	// series, so its Series is the catalog's for that runtime, when the
	// catalog lists it.
	tag    manifest.RuntimeTag
	parsed bool
	// catalogRead says a catalog was there to ask, so an Airflow 2 series
	// still unknown is one the catalog does not list rather than one nobody
	// could look up.
	catalogRead bool
}

// apcBuildOf reads the Dockerfile APC would build, with the catalog the caller
// read, or nil.
func apcBuildOf(dir string, catalog *runtimeversions.Catalog) apcBuild {
	b := apcBuild{base: airflowrt.ReadDeclaredBase(filepath.Join(dir, fileDockerfile)), catalogRead: catalog != nil}
	b.tag, b.parsed = manifest.ParseRuntimeTag(b.base.RuntimeVersion())
	if b.parsed && b.tag.Series == "" && catalog != nil {
		if v := versionForTag(b.tag, b.base.RuntimeVersion(), AirflowPinOptions{Catalog: catalog}); v != b.tag.Major {
			b.tag.Series = v
		}
	}
	return b
}

// disagrees reports a pin this build would not deploy: one the tag does not
// agree with (manifest.RuntimeTag.Agrees), or, where the build's series is
// known, a pin naming only the generation, since "3" against runtime:3.1-12
// resolves to the newest 3.x under `astro local` while APC deploys 3.1. A pin
// to one release of the series ("3.1.2") agrees.
func (b *apcBuild) disagrees(pin string) bool {
	if !b.parsed {
		return false
	}
	return !b.tag.Agrees(pin) || b.tag.Series != "" && !strings.Contains(pin, ".")
}

// seriesUnknown says why an Airflow 2 build's series is not known.
func (b *apcBuild) seriesUnknown() string {
	if b.catalogRead {
		return "the runtime catalog does not list runtime " + b.base.RuntimeVersion()
	}
	return "the runtime catalog, which says which, could not be read"
}

// carries names the Airflow the build carries, for the refusal, and fix is
// the pin that agrees with it.
func (b *apcBuild) carries() (carries, fix string) {
	switch {
	case b.tag.Series != "":
		return "Airflow " + b.tag.Series, "Convert with --airflow-version " + b.tag.Series
	case b.tag.Major == "3":
		return "a floating Airflow 3 tag that names no series", "Convert with --airflow-version 3"
	}
	return "an Airflow 2 runtime whose tag does not name the Airflow series",
		"Convert with --airflow-version set to the Airflow 2 series runtime " + b.base.RuntimeVersion() +
			" carries (" + b.seriesUnknown() + ")"
}

// pinAPCBuildSeries makes the series an APC build carries the pin a
// conversion reads from its Dockerfile. airflowFromDockerfile reads an
// Airflow 2 tag as "2", the newest Airflow 2, because the tag names no minor;
// APC deploys one particular series, so where the catalog named it, that is
// the pin, and where it did not, the note offering "2" as the honest answer
// is replaced by one saying the requirement was not checked, and why.
func pinAPCBuildSeries(from1x *project1x, b *apcBuild) {
	if !b.parsed || from1x.airflow != b.tag.Major {
		return
	}
	i := slices.IndexFunc(from1x.notes, isMinorlessNote)
	switch {
	case b.tag.Series != "":
		from1x.airflow = b.tag.Series
		if i >= 0 {
			from1x.notes = slices.Delete(from1x.notes, i, i+1)
		}
	case i >= 0:
		from1x.notes[i] = "Dockerfile: runtime " + b.base.Tag + " is an Airflow 2 image whose tag does not name the " +
			"Airflow minor, and " + b.seriesUnknown() + ", so the Airflow requirement in " + manifest.Marker +
			" was not checked against it. Astro Private Cloud's `astro deploy` builds that runtime as it stands, so " +
			"pin the requirement to the Airflow series it carries: one naming only the generation follows the newest " +
			"Airflow 2 under `astro local`"
	}
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

// apcBuildNotes is the note naming what survives for APC's build and what
// that asks of the user: one note for any project converted for APC with an
// undeclared Dockerfile, since that file is the build APC deploys whatever
// else kept it, and none otherwise, so a caller appends it unconditionally.
// kept is what planRetirements kept for APC alone.
//
// Each clause is about what it names: pyproject.toml is said to carry the
// same Airflow version only for a Dockerfile kept here (one kept for another
// reason, a note of its own or a release of its series the pin named, is
// named and claims nothing), the lists are said to be installed only by a base
// that installs them, and deleting is offered only for what was kept here,
// which is exactly what would otherwise have retired because the manifest
// carries all of it.
//
// It ends by saying what decided the platform and how to choose the other,
// since that decision is the whole reason the files are still there.
func apcBuildNotes(target *deployTarget, from1x *project1x, kept []string) []string {
	if !target.apc || !from1x.hasDockerfile() || declaresDockerfile(from1x) {
		return nil
	}
	named := kept
	if !slices.Contains(kept, fileDockerfile) {
		named = append([]string{fileDockerfile}, kept...)
	}
	// In the order a manifest states them, whatever order kept came in.
	var lists, same []string
	if slices.Contains(kept, fileDockerfile) {
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
	b.WriteString(joinNames(named) + ": kept for Astro Private Cloud, whose `astro deploy` " +
		"builds the project from its Dockerfile as it stands")
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
	if len(same) > 0 {
		carried := same[len(same)-1]
		if len(same) > 1 {
			carried = strings.Join(same[:len(same)-1], ", ") + " and " + carried
		}
		deletable := pronoun(len(kept))
		if len(named) != len(kept) {
			deletable = joinNames(kept)
		}
		b.WriteString(". " + manifest.Marker + " carries the same " + carried + " for `astro local` and Astro, " +
			"so change both together while the project deploys to Astro Private Cloud, and delete " + deletable +
			" if it deploys to Astro instead")
	}
	if d := target.decided(); d != "" {
		b.WriteString(". " + d)
	}
	return []string{b.String()}
}
