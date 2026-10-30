package imageinstall

import (
	"fmt"
	neturl "net/url"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// The image's policy, applied to what uv says the lock installs: the runtime
// image's packages stay as it ships them; the project's own, and the packages
// the image lacks, get the lock's versions.
//
//   - Airflow and its core are left out: the image is Airflow. So is the Task
//     SDK, unless the project lists it with a version.
//   - So is every package the runtime's constraints file pins (SQLAlchemy, on
//     Airflow 3): the runtime decides those.
//   - A package the project names itself, in dependencies,
//     optional-dependencies or a dependency group, goes into requirements.txt
//     at the lock's version.
//   - Any other package the base image does not already have goes into the
//     lock's constraints file at the lock's version; one it has keeps the
//     runtime's tested version. When what the image has is not known (a
//     runtime that publishes no package list), no constraints are written,
//     and the install keeps whatever the image has that satisfies the
//     requirements, as pip and uv do.
//   - A package from git or a URL is a direct reference to the commit or file
//     the lock recorded, and so is one a [tool.uv.sources] entry routes to an
//     index: the file the lock recorded for the image's platform, by URL and
//     hash, so the image's install never asks its own index for that name.
//     Everything else installs by name through the image's own index, a
//     package from a local path included, which the image does not have.
//   - A user or password in any of those URLs is left out, with one warning:
//     the lists land in an image layer and the build log, and the build
//     authenticates through the netrc secret.
//
// Each line keeps the lock's environment marker, so the image's own installer
// decides, for its own Python and platform, which of them apply.

// Distributions the policy treats by name.
const (
	airflowDist     = "apache-airflow"
	airflowCoreDist = "apache-airflow-core"
	taskSDKDist     = "apache-airflow-task-sdk"
)

// pylock is the part of a PEP 751 pylock.toml the policy reads.
type pylock struct {
	Packages []lockedPackage `toml:"packages"`
}

// lockedPackage is one [[packages]] entry of a pylock.toml.
type lockedPackage struct {
	Name    string       `toml:"name"`
	Version string       `toml:"version"`
	Marker  string       `toml:"marker"`
	VCS     *lockedVCS   `toml:"vcs"`
	Archive *lockedFile  `toml:"archive"`
	Sdist   *lockedFile  `toml:"sdist"`
	Wheels  []lockedFile `toml:"wheels"`
}

// lockedVCS is a package's vcs table: the repository and the commit the lock
// recorded.
type lockedVCS struct {
	Type         string `toml:"type"`
	URL          string `toml:"url"`
	CommitID     string `toml:"commit-id"`
	Subdirectory string `toml:"subdirectory"`
}

// lockedFile is a wheel, an sdist or an archive: where it is and its hashes.
type lockedFile struct {
	URL    string            `toml:"url"`
	Hashes map[string]string `toml:"hashes"`
}

// projectNames are what the policy reads of pyproject.toml: the names the
// project lists itself, with whether it gives a version, and the packages a
// [tool.uv.sources] entry routes to an index.
type projectNames struct {
	// own maps each name the project lists, in dependencies,
	// optional-dependencies or a dependency group, to whether one of its
	// listings states a version.
	own map[string]bool
	// routed are the names a [tool.uv.sources] entry, or one of a list of
	// them, routes to an index.
	routed map[string]bool
}

// readProject reads projectNames from pyproject.toml's bytes.
func readProject(data []byte) (projectNames, error) {
	var doc struct {
		Project struct {
			Dependencies         []string            `toml:"dependencies"`
			OptionalDependencies map[string][]string `toml:"optional-dependencies"`
		} `toml:"project"`
		DependencyGroups map[string][]any `toml:"dependency-groups"`
		Tool             struct {
			UV struct {
				Sources map[string]any `toml:"sources"`
			} `toml:"uv"`
		} `toml:"tool"`
	}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return projectNames{}, fmt.Errorf("reading pyproject.toml: %w", err)
	}
	p := projectNames{own: map[string]bool{}, routed: map[string]bool{}}
	add := func(req string) {
		name := manifest.DistName(req)
		p.own[name] = p.own[name] || statesVersion(req)
	}
	for _, r := range doc.Project.Dependencies {
		add(r)
	}
	for _, rs := range doc.Project.OptionalDependencies {
		for _, r := range rs {
			add(r)
		}
	}
	for _, rs := range doc.DependencyGroups {
		for _, r := range rs {
			// A group can also include another ({include-group = ...}).
			if s, ok := r.(string); ok {
				add(s)
			}
		}
	}
	for name, source := range doc.Tool.UV.Sources {
		entries, ok := source.([]any)
		if !ok {
			entries = []any{source}
		}
		for _, e := range entries {
			if table, ok := e.(map[string]any); ok && table["index"] != nil {
				p.routed[manifest.DistName(name)] = true
			}
		}
	}
	return p, nil
}

// statesVersion reports a requirement that limits its version: a specifier
// after the name and extras, before any marker. A direct reference states
// none.
func statesVersion(req string) bool {
	rest := strings.TrimSpace(req)
	if i := strings.IndexAny(rest, "[ \t<>=!~;@("); i >= 0 {
		rest = rest[i:]
	} else {
		return false
	}
	if strings.HasPrefix(rest, "[") {
		if j := strings.Index(rest, "]"); j >= 0 {
			rest = rest[j+1:]
		}
	}
	rest, _, _ = strings.Cut(rest, ";")
	rest = strings.TrimSpace(rest)
	return rest != "" && strings.ContainsAny(rest[:1], "<>=!~(")
}

// policyInput is what the policy decides from.
type policyInput struct {
	// pyproject is the project's pyproject.toml, and pylock what `uv export
	// --format pylock.toml` printed of its lock.
	pyproject, pylock []byte
	// runtimePins are the lines of the runtime's constraints file, and
	// baseInstalled those of the image's freeze. A nil baseInstalled is not
	// known (not the same as an image with nothing in it).
	runtimePins, baseInstalled []string
	// python is the image's Python, "3.12", for the wheel a routed package
	// installs from; "" when not known.
	python string
	// packages are the project's OS packages ([tool.astro] packages).
	packages []string
}

// apply applies the image's policy to in, returning the lists and the
// warnings to show.
func apply(in *policyInput) (imagebuild.InstallLists, []string, error) {
	project, err := readProject(in.pyproject)
	if err != nil {
		return imagebuild.InstallLists{}, nil, err
	}
	var lock pylock
	if err := toml.Unmarshal(in.pylock, &lock); err != nil {
		return imagebuild.InstallLists{}, nil, fmt.Errorf("reading uv's export of uv.lock: %w", err)
	}
	skip := map[string]bool{airflowDist: true, airflowCoreDist: true}
	for _, line := range in.runtimePins {
		if name := pinName(line); name != "" {
			skip[name] = true
		}
	}
	var installed map[string]bool
	if in.baseInstalled != nil {
		installed = map[string]bool{}
		for _, line := range in.baseInstalled {
			if name := pinName(line); name != "" {
				installed[name] = true
			}
		}
	}

	var lists imagebuild.InstallLists
	urls := &urlCleaner{}
	git := false
	for i := range lock.Packages {
		pkg := &lock.Packages[i]
		name := manifest.DistName(pkg.Name)
		versioned, own := project.own[name]
		switch {
		case skip[name]:
			continue
		case name == taskSDKDist && !versioned:
			continue
		case installed[name] && !own:
			continue // the runtime's, at its tested version
		}
		refs := directRefs(pkg, project.routed[name], in.python, urls)
		switch {
		case own || len(refs) > 0:
			if len(refs) == 0 {
				refs = []string{withMarker(pin(pkg), pkg.Marker)}
			}
			lists.Requirements = append(lists.Requirements, refs...)
		case pkg.Version != "" && installed != nil:
			lists.Constraints = append(lists.Constraints, withMarker(pin(pkg), pkg.Marker))
		}
		git = git || pkg.VCS != nil
	}
	lists.Packages = in.packages
	if git {
		lists.Packages = imagebuild.WithGit(lists.Packages)
	}
	var warnings []string
	if urls.stripped {
		warnings = append(warnings, "the image build leaves out a URL's user and password: give the build a netrc secret instead")
	}
	return lists, warnings, nil
}

// pin is name==version, or the bare name for a package with no version.
func pin(pkg *lockedPackage) string {
	if pkg.Version == "" {
		return pkg.Name
	}
	return pkg.Name + "==" + pkg.Version
}

// withMarker is line limited to the environments marker names, as PEP 508
// writes it; the space before the semicolon keeps it apart from a URL.
func withMarker(line, marker string) string {
	if marker = strings.TrimSpace(marker); marker == "" {
		return line
	}
	return line + " ; " + marker
}

// pinName is the normalized name a constraints or freeze line pins, or ""
// for a comment, an option or a blank.
func pinName(line string) string {
	line, _, _ = strings.Cut(line, "#")
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "-") {
		return ""
	}
	return manifest.DistName(line)
}

// directRefs are the requirement lines that install pkg from where the lock
// says it comes from, or none when it installs by name: a git commit, a URL,
// or, when routed, the file the lock recorded on the index a
// [tool.uv.sources] entry routes it to. A routed package whose best file
// differs between the platforms an image is built for gets a line for each,
// limited to that platform.
func directRefs(pkg *lockedPackage, routed bool, python string, urls *urlCleaner) []string {
	switch {
	case pkg.VCS != nil:
		v := pkg.VCS
		ref := fmt.Sprintf("%s @ %s+%s@%s", pkg.Name, v.Type, urls.clean(v.URL), v.CommitID)
		if v.Subdirectory != "" {
			ref += "#subdirectory=" + v.Subdirectory
		}
		return []string{withMarker(ref, pkg.Marker)}
	case pkg.Archive != nil && pkg.Archive.URL != "":
		return []string{withMarker(pkg.Name+" @ "+urls.clean(pkg.Archive.URL), pkg.Marker)}
	case !routed:
		return nil
	}
	type choice struct {
		machine string
		file    *lockedFile
	}
	var choices []choice
	for _, machine := range imageMachines {
		f := bestFile(pkg, imageTags(python, machine))
		if f == nil {
			// Nothing installs here from the lock's files; by name, then, as
			// the helper did, for every platform alike.
			return nil
		}
		choices = append(choices, choice{machine, f})
	}
	if choices[0].file == choices[1].file {
		return []string{withMarker(fileRef(pkg.Name, choices[0].file, urls), pkg.Marker)}
	}
	refs := make([]string, 0, len(choices))
	for _, c := range choices {
		marker := "platform_machine == '" + c.machine + "'"
		if m := strings.TrimSpace(pkg.Marker); m != "" {
			marker = "(" + m + ") and " + marker
		}
		refs = append(refs, withMarker(fileRef(pkg.Name, c.file, urls), marker))
	}
	return refs
}

// fileRef is `name @ url#sha256=…` for a file the lock recorded.
func fileRef(name string, f *lockedFile, urls *urlCleaner) string {
	ref := name + " @ " + urls.clean(f.URL)
	if sha := f.Hashes["sha256"]; sha != "" {
		ref += "#sha256=" + sha
	}
	return ref
}

// bestFile is the file of pkg an install for tags picks: the wheel whose best
// tag ranks first, else the sdist, else nil.
func bestFile(pkg *lockedPackage, tags map[string]int) *lockedFile {
	var best *lockedFile
	bestRank := len(tags)
	for i := range pkg.Wheels {
		w := &pkg.Wheels[i]
		for _, tag := range wheelTags(w.URL) {
			if rank, ok := tags[tag]; ok && rank < bestRank {
				best, bestRank = w, rank
			}
		}
	}
	if best != nil {
		return best
	}
	return pkg.Sdist
}

// urlCleaner leaves a URL's user and password out, and remembers that it did.
type urlCleaner struct {
	stripped bool
}

// clean is raw without the user and password in its authority.
func (c *urlCleaner) clean(raw string) string {
	u, err := neturl.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	c.stripped = true
	u.User = nil
	return u.String()
}
