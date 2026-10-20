package scaffold

import (
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// A 1.x Dockerfile that does more than name a base is the project's build, and
// its final stage is FROM an Astro Runtime tag whose ONBUILD triggers install
// requirements.txt and packages.txt from the build's context. Kept as it was,
// the conversion has to keep both files too, and the project then has two
// package lists that nothing holds in step: standalone mode installs
// pyproject.toml's, every image the 1.x files'.
//
// So the conversion moves the file onto the same tag's -base image, which has
// no triggers, and writes the install out in front of the file's own steps
// (airflowrt.InstallDockerfile): a stage that reads pyproject.toml and uv.lock
// with the install helper, then the runtime's own install steps. The two 1.x
// files are then carried and nothing reads them, so they go like any other
// carried file, and `docker build .` builds what `astro package astro` does.
//
// The rewrite is all or nothing, and it happens only when the file's shape is
// one this can rewrite without guessing (dockerfileRewriteOf). Anything else
// keeps today's behavior, the file and both lists as they were, with a note
// that names what stopped it.

// dockerfileRewrite is a Dockerfile this conversion rewrites onto a -base tag.
type dockerfileRewrite struct {
	// body is the rewritten file.
	body []byte
	// from is the final stage's reference as the file had it, and base the
	// -base reference it builds on now.
	from, base string
	// mirror reports a base outside Astronomer's own repositories, whose
	// -base tag the mirror has to carry too.
	mirror bool
}

// installBlockComment and ownStepsComment frame the install block in a
// rewritten Dockerfile, so a reader can tell the CLI's lines from the
// project's.
const (
	installBlockComment = "# Installs this project's packages from pyproject.toml and uv.lock\n" +
		"# (.astro/install-project.py reads them; astro init keeps both up to date).\n"
	ownStepsComment = "# This project's own steps.\n"
)

// dockerfileRewriteOf works out the rewrite of a Dockerfile, data, that does
// more than name a base; name is its path, for the notes. It returns the rewrite, or a note naming why the
// file's shape rules one out, or neither when the file is not on an ONBUILD
// tag at all. listsRead reports whether any stage might run ONBUILD steps,
// which read requirements.txt and packages.txt: when none can, the two lists
// are read by nothing whether or not the file is rewritten.
//
// What it can judge from the file alone. Whether the two 1.x files were carried
// is decided later, by plan, from the run's notes.
func dockerfileRewriteOf(name string, data []byte) (rw *dockerfileRewrite, blocker string, listsRead bool) {
	prefix := name + ": kept FROM its Astro Runtime image rather than moved onto that image's -base tag, "
	stages := airflowrt.DockerfileStages(data)
	if len(stages) == 0 {
		return nil, "", false
	}
	for i := range stages {
		listsRead = listsRead || stages[i].MightCarryOnBuild()
	}
	if !listsRead {
		return nil, "", false
	}
	final := stages[len(stages)-1]
	// Any stage on an ONBUILD tag runs the triggers, the final one or not, so
	// the file is rewritable only when the final stage is the one stage that
	// can: an earlier one would still read the 1.x files. Earlier stages are
	// held to what can be shown, not to what is likely: a build argument, a
	// digest or a mirror's floating tag might be an ONBUILD tag, and retiring
	// the lists under one breaks its build.
	for i := range stages[:len(stages)-1] {
		if st := &stages[i]; st.MightCarryOnBuild() {
			return nil, prefix + "because an earlier stage builds on " + st.Ref + ", which astro cannot show is free of the " +
				"ONBUILD steps that read requirements.txt and packages.txt. Build that stage on an image without them, " +
				"such as a -base tag, and run astro init again", true
		}
	}
	switch {
	case airflowrt.IsUnresolvedRef(final.Ref):
		return nil, "", true
	case final.Digest:
		return nil, prefix + "because its FROM pins " + final.Ref + " by digest, and the -base image has a digest of its own: " +
			"to move it, replace the reference with " + final.Image + ":" + nonEmpty(final.Tag, "<tag>") + "-base and add the install steps astro init writes", true
	case !final.OnBuildRuntime():
		return nil, "", true
	}
	// The runtimes astro builds images for, whose -base tags are checked
	// (scripts/check-runtime-base-tags.sh): every Airflow 3 tag, the floating
	// series tags (3.3) among them, and Airflow 2 builds from runtime 9
	// (Airflow 2.7). A floating Airflow 2 tag (12) has no -base twin: none is
	// published. A tag whose version this cannot read is left alone rather
	// than guessed at.
	v, _ := airflowrt.ParseRuntimeTagPython(strings.TrimPrefix(final.Tag, "v"))
	switch t, ok := manifest.ParseRuntimeTag(v); {
	case !ok:
		return nil, prefix + "because astro does not read a runtime version from its tag " + final.Tag, true
	case t.Major == "2" && !t.Build:
		return nil, prefix + "because no -base tag is published for the floating Airflow 2 tag " + final.Tag +
			": pin the runtime build, such as 12.6.0, and run astro init again", true
	case t.Major == "2" && runtimeMajor(v) < minAirflow2Runtime:
		return nil, prefix + "because runtime " + final.Tag + " is older than runtime 9 (Airflow 2.7), the oldest astro builds images for", true
	}
	for i := range stages {
		if strings.EqualFold(stages[i].Alias, airflowrt.InstallStage) {
			return nil, prefix + "because it already has a stage named " + stages[i].Alias + ", the name of the stage that does the install", true
		}
	}
	lines := strings.Split(string(data), "\n")
	if strings.HasSuffix(strings.TrimRight(lines[final.Line], " \t\r"), "\\") {
		return nil, prefix + "because its final FROM line continues onto the next one", true
	}
	for _, name := range []string{"requirements.txt", "packages.txt"} {
		if instructionsName(lines, name) {
			return nil, prefix + "because it reads " + name + " itself, which the rewritten build would not provide", true
		}
	}

	base := airflowrt.BaseOf(final.Ref)
	from := strings.Join(append(append([]string{}, final.Flags...), base), " ")
	install := airflowrt.InstallDockerfileWith(from, airflowrt.InstallOptions{Alias: final.Alias})

	var b strings.Builder
	if final.Line > 0 {
		b.WriteString(strings.Join(lines[:final.Line], "\n"))
		b.WriteString("\n")
	}
	b.WriteString(installBlockComment)
	b.WriteString(install)
	if own := lines[final.Line+1:]; strings.TrimSpace(strings.Join(own, "\n")) != "" {
		b.WriteString("\n" + ownStepsComment)
		// The file's own lines keep their leading blank lines out of the way of
		// the comment, and their trailing newline as the file had it.
		for len(own) > 0 && strings.TrimSpace(own[0]) == "" {
			own = own[1:]
		}
		b.WriteString(strings.Join(own, "\n"))
	}
	body := b.String()
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return &dockerfileRewrite{
		body:   []byte(body),
		from:   final.Ref,
		base:   base,
		mirror: !airflowrt.IsPublishedRuntimeRepo(final.Image),
	}, "", true
}

// instructionsName reports a Dockerfile instruction that names the file name as
// a path of its own: requirements.txt, ./requirements.txt or
// /tmp/requirements.txt, and not dev-requirements.txt, and not in a comment.
// Words are split on whitespace and on the punctuation instructions put
// around paths (quotes, brackets, commas, =).
func instructionsName(lines []string, name string) bool {
	for _, line := range lines {
		if t := strings.TrimSpace(line); t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		for _, word := range strings.FieldsFunc(line, func(r rune) bool {
			return r == ' ' || r == '\t' || strings.ContainsRune("\"'[],=;", r)
		}) {
			if word == name || strings.HasSuffix(word, "/"+name) {
				return true
			}
		}
	}
	return false
}

func nonEmpty(s, otherwise string) string {
	if s == "" {
		return otherwise
	}
	return s
}

// installInputs are the files the install stage of a rewritten Dockerfile reads
// from the build's context, which its ignore file must not leave out.
var installInputs = []string{manifest.Marker, "uv.lock", airflowrt.InstallHelperPath}

// planDockerfileRewrite adds the rewrite to cs when it can go ahead, and
// otherwise the notes that say why the 1.x files stay. notes are the run's
// notes so far, which say whether requirements.txt and packages.txt were
// carried whole: a line either one could not carry is in a note naming it, and
// the file stays as its only record, so the Dockerfile that reads it does too.
func planDockerfileRewrite(dir string, from1x *project1x, notes []string, cs *Changeset) error {
	rw := from1x.rewrite
	if rw == nil {
		from1x.notes = append(from1x.notes, keptListNotes(from1x)...)
		return nil
	}
	for _, name := range []string{"requirements.txt", "packages.txt"} {
		if slices.Contains(from1x.present, name) && namedInAny(notes, name) {
			from1x.rewrite = nil
			from1x.notes = append(from1x.notes, onBuildKeptNote("Dockerfile: kept FROM "+rw.from+" rather than moved onto "+rw.base+
				", because "+name+" has lines "+manifest.Marker+" does not carry (above), and the base's ONBUILD steps "+
				"still install them from it. Once they are in "+manifest.Marker+", run astro init again to move it"))
			from1x.notes = append(from1x.notes, keptListNotes(from1x)...)
			return nil
		}
	}
	switch blocker, err := planHelperCommittable(dir, fileDockerfile, cs); {
	case err != nil:
		return err
	case blocker != "":
		from1x.rewrite = nil
		from1x.notes = append(from1x.notes, onBuildKeptNote(blocker))
		from1x.notes = append(from1x.notes, keptListNotes(from1x)...)
		return nil
	}
	label := fileDockerfile + " (now builds FROM " + rw.base + " and installs this project's packages from " +
		manifest.Marker + " and uv.lock, so docker build . and astro deploy build the same image)"
	helper := Change{
		Kind:    CreateFile,
		Path:    airflowrt.InstallHelperPath,
		Content: airflowrt.InstallHelper(),
		Labels:  []string{airflowrt.InstallHelperPath + " (reads " + manifest.Marker + " and uv.lock inside the build)"},
	}
	cs.Changes = append(cs.Changes, Change{Kind: UpdateFile, Path: fileDockerfile, Content: rw.body, Labels: []string{label}}, helper)
	if rw.mirror {
		cs.Advisories = append(cs.Advisories, fileDockerfile+" now builds FROM "+rw.base+
			": if your registry mirrors "+rw.from+" tag by tag, mirror "+rw.base+" too")
	}
	return nil
}

// planDockerfile is what a conversion does about a Dockerfile it keeps as the
// build: the move onto -base when it can go ahead (planDockerfileRewrite), then
// the ignore file the build's context needs (planKeptDockerfileIgnore), which
// depends on whether the move went ahead.
func planDockerfile(dir string, from1x *project1x, notes []string, cs *Changeset) error {
	if err := planDockerfileRewrite(dir, from1x, notes, cs); err != nil {
		return err
	}
	return planKeptDockerfileIgnore(dir, from1x, cs)
}

// keptListNotes are the notes for a declared Dockerfile kept on its ONBUILD
// tag: both 1.x lists stay, because the base's ONBUILD steps read them.
func keptListNotes(from1x *project1x) []string {
	if !declaresDockerfile(from1x) || !from1x.listsRead {
		return nil
	}
	var notes []string
	for _, kept := range []string{"requirements.txt", "packages.txt"} {
		if slices.Contains(from1x.present, kept) {
			notes = append(notes, kept+": kept, because your Dockerfile's base image reads it "+
				"during the build. Its contents are in pyproject.toml as well, which is what a project "+
				"without a Dockerfile installs from")
		}
	}
	return notes
}
