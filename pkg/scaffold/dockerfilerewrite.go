package scaffold

import (
	"path/filepath"
	"regexp"
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
// So the conversion moves the file onto the install base the CLI builds from
// pyproject.toml and uv.lock: its final stage becomes FROM ${ASTRO_BASE}
// (airflowrt.BaseArg), followed by the step that lays the project's files over
// it, as the ONBUILD tag's last trigger did, and then the file's own steps.
// The runtime the tag named moves into pyproject.toml ([tool.astro] runtime
// for one build, requires-python for a Python flavor, [tool.astro] registry
// for a mirror), so the runtime has one source and the file names none. The
// two 1.x files are then carried and nothing reads them, so they go like any
// other carried file. A plain `docker build .` has no ASTRO_BASE and fails at
// that FROM; the file's header says how to build it (astro package, astro
// deploy, or `astro package --context-dir`, which writes both builds and
// prints the docker commands).
//
// The rewrite is all or nothing, and it happens only when the file's shape is
// one this can rewrite without guessing (dockerfileRewriteOf). Anything else
// keeps today's behavior, the file and both lists as they were, with a note
// that names what stopped it.

// dockerfileRewrite is a Dockerfile this conversion moves onto the install
// base.
type dockerfileRewrite struct {
	// body is the rewritten file.
	body []byte
	// from is the final stage's reference as the file had it.
	from string
	// runtime is the one runtime build the tag named ("3.3-8", "12.6.0"), for
	// [tool.astro] runtime, or "" for a series tag (3.3), which the
	// requirement's series already picks.
	runtime string
	// registry is the mirror the tag was on (registry.example/astronomer), for
	// [tool.astro] registry, or "" for Astronomer's own repositories.
	registry string
	// python is the Python a -python-X.Y flavor named, for requires-python,
	// or "".
	python string
}

// installBaseRuntimeOf reads what pyproject.toml has to record of final, a
// stage on an Astro Runtime tag (an ONBUILD tag, or its -base twin), for a
// file that builds on the install base instead: the one build its tag names,
// the mirror it is on and the Python its flavor names (dockerfileRewrite's
// fields). blocker, which starts with prefix, says why the tag cannot be
// recorded: a version this cannot read, a flavor pyproject.toml has no
// spelling for, a floating Airflow 2 tag, or a runtime astro builds no image
// for.
func installBaseRuntimeOf(final *airflowrt.DockerfileStage, prefix string) (rw dockerfileRewrite, blocker string) {
	raw := strings.TrimSuffix(strings.TrimPrefix(final.Tag, "v"), "-base")
	noPython := raw
	if loc := airflowrt.RuntimePythonRe.FindStringIndex(raw); loc != nil {
		noPython = raw[:loc[0]]
	}
	v, python := airflowrt.ParseRuntimeTagPython(raw)
	t, ok := manifest.ParseRuntimeTag(v)
	switch {
	case !ok:
		return rw, prefix + "because astro does not read a runtime version from its tag " + final.Tag
	case noPython != v:
		return rw, prefix + "because its tag " + final.Tag + " names the " + strings.TrimPrefix(noPython, v+"-") +
			" flavor, which pyproject.toml has no way to pick"
	case t.Major == "2" && !t.Build:
		return rw, prefix + "because the floating Airflow 2 tag " + final.Tag + " names no runtime build" +
			": pin the runtime build, such as 12.6.0, and run astro init again"
	case t.Major == "2" && runtimeMajor(v) < minAirflow2Runtime:
		return rw, prefix + "because runtime " + final.Tag + " is older than runtime 9 (Airflow 2.7), the oldest astro builds images for"
	}
	rw.from, rw.python = final.Ref, python
	rw.registry, _ = airflowrt.RegistryOf(final.Image)
	if t.Build {
		rw.runtime = v
	}
	return rw, ""
}

// ownStepsComment marks where a rewritten Dockerfile's own steps start, so a
// reader can tell the CLI's lines from the project's.
const ownStepsComment = "# This project's own steps.\n"

// parserDirectiveRe is a Dockerfile parser directive (# syntax=…, # check=…),
// which docker reads only from the file's first lines, before any other
// comment or instruction.
var parserDirectiveRe = regexp.MustCompile(`^#\s*([a-zA-Z]+)\s*=`)

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
	prefix := name + ": kept FROM its Astro Runtime image rather than moved onto the install base the Astro CLI builds, "
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
		return nil, prefix + "because its FROM pins " + final.Ref + " by digest, which names an image rather than a runtime build " +
			"pyproject.toml can record. Name the tag instead and run astro init again", true
	case !final.OnBuildRuntime():
		return nil, "", true
	}
	// The runtimes astro builds images for: every Airflow 3 tag, the floating
	// series tags (3.3) among them, and Airflow 2 builds from runtime 9
	// (Airflow 2.7). A floating Airflow 2 tag (12) names no build, and a flavor
	// other than a Python one (-slim, -ubi9) has no pyproject.toml spelling, so
	// either is left as it is rather than guessed at.
	recorded, blocker := installBaseRuntimeOf(&final, prefix)
	if blocker != "" {
		return nil, blocker, true
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
	recorded.body = rewrittenBody(lines, &final, final.Line, final.Line+1)
	return &recorded, "", true
}

// rewrittenBody is a Dockerfile's lines with its final stage, final, moved
// onto the install base: the file's parser directives, with the one that
// silences BuildKit's warning about an argument with no default; the header
// and the global ARG, ahead of any earlier stage; the file's lines before
// cut, the lines from cut to resume replaced by the final FROM on
// ${ASTRO_BASE}, its flags and name kept, and the step that lays down the
// project's files; then the file's own lines from resume on. A conversion cuts
// the final FROM line alone; an update cuts the install block an older astro
// wrote around it.
func rewrittenBody(lines []string, final *airflowrt.DockerfileStage, cut, resume int) []byte {
	var b strings.Builder
	head := 0
	hasCheck := false
	for head < len(lines) && head < cut {
		m := parserDirectiveRe.FindStringSubmatch(strings.TrimSpace(lines[head]))
		if m == nil {
			break
		}
		hasCheck = hasCheck || strings.EqualFold(m[1], "check")
		b.WriteString(lines[head] + "\n")
		head++
	}
	if !hasCheck {
		b.WriteString(airflowrt.BaseArgCheckDirective + "\n")
	}
	b.WriteString(airflowrt.BaseArgComment)
	b.WriteString("ARG " + airflowrt.BaseArg + "\n")
	if cut > head {
		b.WriteString(strings.Join(lines[head:cut], "\n") + "\n")
	}
	from := append(append([]string{"FROM"}, final.Flags...), airflowrt.BaseArgFrom)
	if final.Alias != "" {
		from = append(from, "AS", final.Alias)
	}
	b.WriteString(strings.Join(from, " ") + "\n")
	b.WriteString(airflowrt.ProjectCopy + "\n")
	if own := lines[resume:]; strings.TrimSpace(strings.Join(own, "\n")) != "" {
		b.WriteString("\n" + ownStepsComment)
		// The file's own lines keep their leading blank lines, and a comment
		// an older astro wrote ahead of them, out of the way of the comment,
		// and their trailing newline as the file had it.
		for len(own) > 0 && (strings.TrimSpace(own[0]) == "" || own[0]+"\n" == ownStepsComment) {
			own = own[1:]
		}
		b.WriteString(strings.Join(own, "\n"))
	}
	body := b.String()
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return []byte(body)
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

// staleHelperPath is where Astro CLI preview builds wrote the install helper
// a rewritten Dockerfile ran inside the build, and staleHelperHeader the line
// each of them began it with. The CLI resolves the install itself now, so a
// project carrying one has no use for it.
const (
	staleHelperPath   = ".astro/install-project.py"
	staleHelperHeader = "# Written by the Astro CLI. Do not edit: `astro init` replaces this file."
)

// planStaleHelperRemoval removes the install helper an Astro CLI preview build
// wrote into the project, when the file is there and is the CLI's own (its
// first line says so); a file of the project's own at that path is left
// alone.
func planStaleHelperRemoval(dir string, cs *Changeset) error {
	data, err := readIfPresent(filepath.Join(dir, filepath.FromSlash(staleHelperPath)))
	if err != nil || data == nil {
		return err
	}
	first, _, _ := strings.Cut(string(data), "\n")
	if strings.TrimRight(first, "\r") != staleHelperHeader {
		return nil
	}
	cs.Changes = append(cs.Changes, Change{
		Kind: Delete,
		Path: staleHelperPath,
		Labels: []string{staleHelperPath + " (written by an earlier Astro CLI preview, removed: " +
			"the CLI resolves the install before the build now)"},
	})
	return nil
}

// planDockerfileRewrite adds the rewrite to cs when it can go ahead, and
// otherwise the notes that say why the 1.x files stay. notes are the run's
// notes so far, which say whether requirements.txt and packages.txt were
// carried whole: a line either one could not carry is in a note naming it, and
// the file stays as its only record, so the Dockerfile that reads it does too.
func planDockerfileRewrite(from1x *project1x, notes []string, cs *Changeset) {
	rw := from1x.rewrite
	if rw == nil {
		from1x.notes = append(from1x.notes, keptListNotes(from1x)...)
		return
	}
	for _, name := range []string{"requirements.txt", "packages.txt"} {
		if slices.Contains(from1x.present, name) && namedInAny(notes, name) {
			from1x.rewrite = nil
			from1x.notes = append(from1x.notes, onBuildKeptNote("Dockerfile: kept FROM "+rw.from+" rather than moved onto the install base the Astro CLI builds"+
				", because "+name+" has lines "+manifest.Marker+" does not carry (above), and the base's ONBUILD steps "+
				"still install them from it. Once they are in "+manifest.Marker+", run astro init again to move it"))
			from1x.notes = append(from1x.notes, keptListNotes(from1x)...)
			return
		}
	}
	label := fileDockerfile + " (now builds on the install base the Astro CLI makes from " + manifest.Marker +
		" and uv.lock; build it with astro package or astro deploy, or astro package --context-dir for docker build)"
	cs.Changes = append(cs.Changes, Change{Kind: UpdateFile, Path: fileDockerfile, Content: rw.body, Labels: []string{label}})
	if rw.registry != "" {
		cs.Advisories = append(cs.Advisories, manifest.Marker+" records your mirror as [tool.astro] registry = '"+rw.registry+
			"': the install base builds FROM the runtime's -base tag there, so if your registry mirrors "+rw.from+" tag by tag, mirror its -base tag too")
	}
}

// planDockerfile is what a conversion does about the Dockerfile: the move onto
// -base when it can go ahead (planDockerfileRewrite), the removal of the
// install helper an earlier preview wrote (planStaleHelperRemoval), then the
// ignore file the build's context needs (planKeptDockerfileIgnore).
func planDockerfile(dir string, from1x *project1x, notes []string, cs *Changeset) error {
	planDockerfileRewrite(from1x, notes, cs)
	if err := planStaleHelperRemoval(dir, cs); err != nil {
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
