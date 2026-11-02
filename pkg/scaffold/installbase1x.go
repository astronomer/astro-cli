package scaffold

import (
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// A 1.x Dockerfile that does more than name a base builds its final stage on
// an Astro Runtime ONBUILD tag, whose triggers install requirements.txt and
// packages.txt from the build context. Kept as it was, the project would have
// two package lists: pyproject.toml's for standalone mode, the 1.x files'
// for every image.
//
// So a conversion moves the final stage onto the install base the CLI builds
// from pyproject.toml and uv.lock (FROM localhost/astro-base), followed by the
// project's files, as the ONBUILD tag's last trigger laid them down, and then
// the file's own steps unchanged. The runtime the tag named moves into
// pyproject.toml: [tool.astro] runtime for one build, requires-python for a
// Python flavor, [tool.astro] registry for a mirror. The rewrite happens only
// for a shape it needs no guess for; any other file is left as it is.

// installBaseHeader is what a rewritten Dockerfile says above its FROM, where
// BuildKit's error for a missing base shows it.
const installBaseHeader = "# Built on the install base the Astro CLI makes from pyproject.toml and uv.lock.\n" +
	"# Build with astro package or astro deploy, or run astro package --base first.\n"

// The 1.x package lists an ONBUILD tag's triggers install.
const (
	fileRequirements = "requirements.txt"
	filePackages     = "packages.txt"
)

// projectCopy lays the project's files over the install base, as the ONBUILD
// tag's last trigger did.
const projectCopy = "COPY --chown=astro:0 . ."

// recordedRuntime is what pyproject.toml records of a runtime tag: the one
// build it names ("3.3-8", "12.6.0"), or "" for a series tag; the mirror it is
// on (registry.example/astronomer), or "" for Astronomer's own; and the Python
// a -python-X.Y flavor names, or "".
type recordedRuntime struct {
	runtime, registry, python string
}

// dockerfileRewrite is a Dockerfile this conversion moves onto the install
// base: the rewritten file and what pyproject.toml records of the tag it was on.
type dockerfileRewrite struct {
	recordedRuntime
	body []byte
}

// recordRuntime reads what pyproject.toml has to hold of an Astro Runtime
// image, image:tag, for the project to build on it without naming it. why says
// what stops it: a flavor pyproject.toml has no spelling for, a tag that
// names no runtime build it can read, or a mirror whose repository is named
// unlike Astronomer's.
func recordRuntime(image, tag string) (rec recordedRuntime, why string) {
	raw := strings.TrimPrefix(tag, "v")
	noPython := raw
	if loc := airflowrt.RuntimePythonRe.FindStringIndex(raw); loc != nil {
		noPython = raw[:loc[0]]
	}
	v, python := airflowrt.ParseRuntimeTagPython(raw)
	t, ok := manifest.ParseRuntimeTag(v)
	switch {
	case !ok:
		return rec, "astro does not read a runtime version from its tag " + tag
	case noPython != v:
		return rec, "its tag " + tag + " names the " + strings.TrimPrefix(noPython, v+"-") + " flavor, which pyproject.toml has no way to pick"
	case t.Major == "2" && !t.Build:
		return rec, "the floating Airflow 2 tag " + tag + " names no runtime version: name one, such as 12.6.0, and run astro init again"
	case t.Major == "2" && runtimeMajor(v) < minAirflow2Runtime:
		return rec, "runtime " + tag + " is older than runtime 9 (Airflow 2.7), the oldest astro builds images for"
	}
	repo := airflow3RuntimeImage[strings.LastIndex(airflow3RuntimeImage, "/")+1:]
	if t.Major == "2" {
		repo = airflow2RuntimeImage[strings.LastIndex(airflow2RuntimeImage, "/")+1:]
	}
	if slash := strings.LastIndex(image, "/"); image != airflow2RuntimeImage && image != airflow3RuntimeImage {
		if image[slash+1:] != repo {
			return rec, "its mirror " + image + " is not named " + repo + ", as Astronomer's repository for this Airflow is"
		}
		rec.registry = image[:slash]
	}
	if t.Build {
		rec.runtime = v
	}
	rec.python = python
	return rec, ""
}

// airflow3RuntimeImage is the repository a generated Airflow 3 build pulls its
// base from (pkg/imagebuild's RuntimeImageRepo).
const airflow3RuntimeImage = "astrocrpublic.azurecr.io/runtime"

// fromLine is one FROM instruction of a Dockerfile: its line, flags,
// reference and stage name.
type fromLine struct {
	line         int
	flags        []string
	ref, alias   string
	continuation bool
}

// fromLines are a Dockerfile's FROM instructions, in order.
func fromLines(lines []string) []fromLine {
	var out []fromLine
	for i, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.EqualFold(fields[0], "FROM") {
			continue
		}
		f := fromLine{line: i, continuation: strings.HasSuffix(strings.TrimRight(line, " \t\r"), "\\")}
		rest := fields[1:]
		for len(rest) > 0 && strings.HasPrefix(rest[0], "--") {
			f.flags, rest = append(f.flags, rest[0]), rest[1:]
		}
		if len(rest) > 0 {
			f.ref = rest[0]
		}
		if len(rest) >= 3 && strings.EqualFold(rest[1], "AS") {
			f.alias = rest[2]
		}
		out = append(out, f)
	}
	return out
}

// onBuildTag reports a reference on an Astro Runtime tag whose ONBUILD
// triggers run: a Runtime image whose tag is not -base.
func onBuildTag(ref string) bool {
	image, tag := splitRef(ref)
	return airflowrt.IsAstroRuntimeRef(image, tag) && !strings.Contains(tag, "-base")
}

// splitRef is a reference's image and tag, "" for none.
func splitRef(ref string) (image, tag string) {
	name, _, _ := strings.Cut(ref, "@")
	if colon := strings.LastIndex(name, ":"); colon > strings.LastIndex(name, "/") {
		return name[:colon], name[colon+1:]
	}
	return name, ""
}

// installBaseRewriteOf works out the move onto the install base of a
// Dockerfile, data, that does more than name a base. It returns the rewrite,
// or why the file is left as it is, or neither when its final stage is not on
// an ONBUILD tag (a -base tag, an earlier stage's name, a build argument),
// which nothing here rewrites or needs to explain.
func installBaseRewriteOf(data []byte) (rw *dockerfileRewrite, why string) {
	lines := strings.Split(string(data), "\n")
	froms := fromLines(lines)
	if len(froms) == 0 {
		return nil, ""
	}
	final := froms[len(froms)-1]
	image, tag := splitRef(final.ref)
	switch {
	case airflowrt.IsUnresolvedRef(final.ref) || !airflowrt.IsAstroRuntimeRef(image, tag):
		return nil, ""
	case strings.Contains(final.ref, "@"):
		return nil, "its FROM pins " + final.ref + " by digest, which names an image rather than a runtime version"
	case !onBuildTag(final.ref):
		return nil, ""
	case final.continuation:
		return nil, "its final FROM line continues onto the next one"
	}
	for _, f := range froms[:len(froms)-1] {
		if airflowrt.IsUnresolvedRef(f.ref) || onBuildTag(f.ref) {
			return nil, "an earlier stage builds on " + f.ref + ", whose ONBUILD steps read requirements.txt and packages.txt"
		}
	}
	for _, name := range []string{fileRequirements, filePackages} {
		if instructionsName(lines, name) {
			return nil, "it reads " + name + " itself"
		}
	}
	rec, why := recordRuntime(image, tag)
	if why != "" {
		return nil, why
	}
	var b strings.Builder
	if final.line > 0 {
		b.WriteString(strings.Join(lines[:final.line], "\n") + "\n")
	}
	b.WriteString(installBaseHeader)
	from := append(append([]string{"FROM"}, final.flags...), airflowrt.InstallBaseRepo)
	if final.alias != "" {
		from = append(from, "AS", final.alias)
	}
	b.WriteString(strings.Join(from, " ") + "\n" + projectCopy + "\n")
	own := lines[final.line+1:]
	for len(own) > 0 && strings.TrimSpace(own[0]) == "" {
		own = own[1:]
	}
	if len(own) > 0 {
		b.WriteString("\n" + strings.Join(own, "\n"))
	}
	body := b.String()
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return &dockerfileRewrite{recordedRuntime: rec, body: []byte(body)}, ""
}

// planRewrite decides the move onto the install base of the project's
// Dockerfile, data: the rewrite and what pyproject.toml records of its tag, or
// a note naming why the file is left as it is. A requirements.txt or
// packages.txt with lines pyproject.toml could not hold keeps the file on its
// ONBUILD tag, which still installs them.
func (from1x *project1x) planRewrite(data []byte) {
	rw, why := installBaseRewriteOf(data)
	for _, list := range []string{fileRequirements, filePackages} {
		if rw != nil && slices.Contains(from1x.present, list) && namedInAny(from1x.notes, list) {
			rw, why = nil, list+" has lines "+manifest.Marker+" cannot hold (above)"
		}
	}
	switch {
	case rw != nil:
		from1x.rewrite, from1x.runtime, from1x.registry = rw, rw.runtime, rw.registry
	case why != "":
		from1x.notes = append(from1x.notes, fileDockerfile+": left as it is, because "+why+
			". It builds as written, and its base image still installs "+fileRequirements+" and "+filePackages)
	}
}

// rewriteChange is the change that writes a rewritten Dockerfile.
func rewriteChange(rw *dockerfileRewrite) Change {
	return Change{
		Kind:    UpdateFile,
		Path:    fileDockerfile,
		Content: rw.body,
		Labels:  []string{fileDockerfile + " (now builds on the install base the Astro CLI makes from " + manifest.Marker + " and uv.lock)"},
	}
}

// instructionsName reports a Dockerfile instruction that names the file name
// as a path of its own (requirements.txt, ./requirements.txt), not in a
// comment.
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
