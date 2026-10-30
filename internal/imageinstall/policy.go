package imageinstall

import (
	"errors"
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
//     the lock recorded. One a [tool.uv.sources] entry routes to an index
//     installs before everything else, from the files the lock recorded for
//     it alone (InstallLists.Routed): pip, in the image, picks the one its
//     Python and platform take, and checks it against the lock's hash, so
//     the image's install never asks its own index for that name. It is in
//     requirements.txt too, at the same version, which the main install
//     finds already installed, so its dependencies install with the rest.
//     Everything else installs by name through the image's own index. A
//     package from a path on this machine, a path source or a workspace
//     member, is refused (ErrLocalSource): the image cannot reach it.
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
	Name      string       `toml:"name"`
	Version   string       `toml:"version"`
	Marker    string       `toml:"marker"`
	VCS       *lockedVCS   `toml:"vcs"`
	Archive   *lockedFile  `toml:"archive"`
	Directory *lockedDir   `toml:"directory"`
	Sdist     *lockedFile  `toml:"sdist"`
	Wheels    []lockedFile `toml:"wheels"`
}

// lockedVCS is a package's vcs table: the repository and the commit the lock
// recorded.
type lockedVCS struct {
	Type         string `toml:"type"`
	URL          string `toml:"url"`
	CommitID     string `toml:"commit-id"`
	Subdirectory string `toml:"subdirectory"`
}

// lockedFile is a wheel, an sdist or an archive: where it is, a URL or a
// path on this machine, and its hashes.
type lockedFile struct {
	URL    string            `toml:"url"`
	Path   string            `toml:"path"`
	Hashes map[string]string `toml:"hashes"`
}

// lockedDir is a package's directory table: a source tree on this machine, a
// path source or a workspace member.
type lockedDir struct {
	Path string `toml:"path"`
}

// localPath is where on this machine the lock takes pkg from, a path source
// or a workspace member, or "" when it comes from an index, git or a URL.
func localPath(pkg *lockedPackage) string {
	switch {
	case pkg.Directory != nil:
		return pkg.Directory.Path
	case pkg.Archive != nil && pkg.Archive.URL == "":
		return pkg.Archive.Path
	}
	return ""
}

// ErrLocalSource reports a package the lock takes from a path on this
// machine, which an image cannot install: its install reads indexes, git and
// URLs, before the project's files are in it.
var ErrLocalSource = errors.New("the image cannot install a package from a path on this machine")

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
	skip := pinNames(in.runtimePins)
	skip[airflowDist], skip[airflowCoreDist] = true, true
	var installed map[string]bool
	if in.baseInstalled != nil {
		installed = pinNames(in.baseInstalled)
	}

	// leftOut reports a package the image does not install: Airflow, what
	// the runtime pins, a Task SDK the project gives no version, and what
	// the image has that the project does not name, at the runtime's tested
	// version.
	leftOut := func(name string) bool {
		versioned, own := project.own[name]
		return skip[name] || (name == taskSDKDist && !versioned) || (installed[name] && !own)
	}
	if err := refuseLocal(&lock, leftOut); err != nil {
		return imagebuild.InstallLists{}, nil, err
	}
	var lists imagebuild.InstallLists
	urls := &urlCleaner{}
	git := false
	for i := range lock.Packages {
		pkg := &lock.Packages[i]
		name := manifest.DistName(pkg.Name)
		if leftOut(name) {
			continue
		}
		_, own := project.own[name]
		refs := directRefs(pkg, urls)
		switch {
		case len(refs) == 0 && project.routed[name] && hasFiles(pkg):
			// Installed first, alone and without its dependencies, from the
			// lock's files; then named in requirements.txt too, at the same
			// version, so the main install finds it there and installs the
			// dependencies it needs that the image lacks, at the lock's
			// versions.
			line := withMarker(pin(pkg), pkg.Marker)
			lists.Routed = append(lists.Routed, line)
			lists.RoutedLinks = append(lists.RoutedLinks, fileLinks(pkg, urls)...)
			lists.Requirements = append(lists.Requirements, line)
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

// refuseLocal is ErrLocalSource for the packages the image installs (not
// leftOut) that lock takes from a path on this machine (localPath), each
// named with its path, or nil for none.
func refuseLocal(lock *pylock, leftOut func(name string) bool) error {
	var local []string
	for i := range lock.Packages {
		if leftOut(manifest.DistName(lock.Packages[i].Name)) {
			continue
		}
		if p := localPath(&lock.Packages[i]); p != "" {
			local = append(local, fmt.Sprintf("%s (%s)", lock.Packages[i].Name, p))
		}
	}
	if len(local) == 0 {
		return nil
	}
	return fmt.Errorf("%w: uv.lock takes %s from a path, which the image build has no way to reach. "+
		"An image installs packages from an index, a git repository or a URL: publish it to an index and route it there with [tool.uv.sources], or point its source at git or a URL",
		ErrLocalSource, strings.Join(local, ", "))
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

// pinNames are the names lines pin (pinName), as a set.
func pinNames(lines []string) map[string]bool {
	names := map[string]bool{}
	for _, line := range lines {
		if name := pinName(line); name != "" {
			names[name] = true
		}
	}
	return names
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
// says it comes from, or none when it installs by name: a git commit or a
// URL.
func directRefs(pkg *lockedPackage, urls *urlCleaner) []string {
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
	}
	return nil
}

// hasFiles reports a package the lock recorded a wheel or an sdist of. A
// routed package with none installs by name, as any other.
func hasFiles(pkg *lockedPackage) bool {
	return len(pkg.Wheels) > 0 || pkg.Sdist != nil
}

// fileLinks are the links to every wheel and the sdist the lock recorded of
// pkg, each `url#sha256=…`, for the page a routed install finds them on.
func fileLinks(pkg *lockedPackage, urls *urlCleaner) []string {
	files := make([]*lockedFile, 0, len(pkg.Wheels)+1)
	for i := range pkg.Wheels {
		files = append(files, &pkg.Wheels[i])
	}
	if pkg.Sdist != nil {
		files = append(files, pkg.Sdist)
	}
	links := make([]string, 0, len(files))
	for _, f := range files {
		link := urls.clean(f.URL)
		if sha := f.Hashes["sha256"]; sha != "" {
			link += "#sha256=" + sha
		}
		links = append(links, link)
	}
	return links
}

// urlCleaner leaves a URL's user and password out, and remembers that it did.
type urlCleaner struct {
	stripped bool
}

// clean is raw without the user and password in its authority. An ssh URL's
// user without a password (ssh://git@github.com/…) is kept: it is the account
// ssh logs in as, not a secret, and the clone fails without it. A user on any
// other URL can be a token, so it goes.
func (c *urlCleaner) clean(raw string) string {
	u, err := neturl.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	if _, hasPassword := u.User.Password(); !hasPassword && strings.HasSuffix(u.Scheme, "ssh") {
		return raw
	}
	c.stripped = true
	u.User = nil
	return u.String()
}
