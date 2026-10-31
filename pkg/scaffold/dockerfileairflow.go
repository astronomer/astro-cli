package scaffold

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// A project that declares its own Dockerfile states its Airflow twice: the
// requirement in [project] dependencies, which standalone installs, and the
// Dockerfile's FROM line, which Docker mode, deploy and package build. When
// the FROM names an Astro Runtime of another Airflow, the two modes run
// different versions and nothing says so, which is the split the requirement
// exists to end. So that is refused wherever the project runs, in both modes:
// `astro local start` (standalone too, since standalone is exactly where the
// requirement is what runs), astro deploy, astro package and astro local check.
// Astro Desktop calls the same function on its own run paths.
//
// It reads a file, so it is not part of manifest.Parse, which stays pure. The
// comparison itself is manifest.DockerfileRuntimeProblem; the FROM line is read
// by airflowrt.DeclaredBase, the reader Docker mode's own base checks use.

// dockerfilePath is the declared Dockerfile's path on disk, "" when the
// manifest declares none.
func dockerfilePath(dir string, m *manifest.Manifest) string {
	if m == nil || m.Astro.Dockerfile == "" {
		return ""
	}
	return filepath.Join(dir, filepath.FromSlash(m.Astro.Dockerfile))
}

// declaredOwnBase reports a declared Dockerfile whose FROM line picks the
// image, rather than one built on the install base (airflowrt.UsesInstallBase),
// whose base follows pyproject.toml. A file that cannot be read picks its own,
// as far as this can tell.
func declaredOwnBase(dir string, m *manifest.Manifest) bool {
	path := dockerfilePath(dir, m)
	if path == "" {
		return false
	}
	data, err := os.ReadFile(path)
	return err != nil || !airflowrt.UsesInstallBase(data)
}

// CheckDockerfileAirflow refuses a declared Dockerfile whose FROM line names an
// Astro Runtime of another Airflow than the manifest's requirement: another
// series for an Airflow 3 tag, another generation for an Airflow 2 one. dir is
// the project root and m its loaded manifest.
//
// The refusal is a *manifest.ValidationError carrying one problem with
// manifest.CodeDockerfileAirflowMismatch, keyed tool.astro.dockerfile, so a
// caller shows and branches on it as it does any other manifest problem; the
// fix it offers is MatchAirflowToDockerfile.
//
// A FROM that cannot be compared passes: no declared Dockerfile, a file that
// cannot be read (the build reports that, naming the path), a base built from a
// build argument, one pinned by digest, one that is not an Astro Runtime image,
// or a tag the runtime grammar does not read (an untagged FROM among them).
func CheckDockerfileAirflow(dir string, m *manifest.Manifest) error {
	return CheckDockerfileAirflowWith(dir, m, nil)
}

// CheckDockerfileAirflowWith is CheckDockerfileAirflow that also reads, from
// the catalog catalog returns, the Airflow release an Airflow 2 base carries.
// catalog is called only for such a base, so every other project reads none. An Airflow 2 tag
// ("12.6.0") names only the generation, so without the catalog a 2.11
// requirement passes against a 2.10 image; with it, the release the build
// carries has to be one the requirement covers. A nil catalog, a floating or
// flavored tag, or a build the catalog does not list is judged by the tag
// alone, as before.
func CheckDockerfileAirflowWith(dir string, m *manifest.Manifest, catalog func() *runtimeversions.Catalog) error {
	path := dockerfilePath(dir, m)
	if path == "" {
		return nil
	}
	base := airflowrt.ReadDeclaredBase(path)
	p, ok := m.DockerfileRuntimeProblem(base.Ref(), base.RuntimeVersion())
	if !ok && catalog != nil {
		if t, parsed := manifest.ParseRuntimeTag(base.RuntimeVersion()); parsed && t.Major == "2" && t.Build {
			if r, listed := catalogRuntime(catalog(), base.RuntimeVersion()); listed {
				p, ok = m.DockerfileReleaseProblem(base.Ref(), r.AirflowVersion)
			}
		}
	}
	if !ok {
		return nil
	}
	return &manifest.ValidationError{Path: filepath.Join(dir, manifest.Marker), Problems: []manifest.Problem{p}}
}

// CheckDockerfileOnBuild judges a declared Dockerfile with a stage on an Astro
// Runtime ONBUILD tag, whose image installs requirements.txt and packages.txt
// rather than the manifest's lists, so it and standalone mode run different
// packages. dir is the project root and m its loaded manifest.
//
// It refuses only what astro init can fix. When init, run in the project,
// would move the file onto its -base tag (the update step, with every gate a
// conversion applies), the build is refused: err wraps ErrOnBuildBase
// and carries the lines that fix it, and naming astro init in it is a promise
// that command keeps. When a gate stops the move, the build goes ahead on the
// ONBUILD tag as it always has, the triggers still reading the two lists, and
// warning is what init would say about it, for the build to show each time.
// Both are empty for a Dockerfile on no ONBUILD tag.
//
// Every path that builds the file calls it: astro local check and astro
// package, astro deploy, and a Docker-mode start or restart. Not standalone
// mode, which builds no image.
func CheckDockerfileOnBuild(dir string, m *manifest.Manifest) (warning string, err error) {
	path := dockerfilePath(dir, m)
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil //nolint:nilerr // a file that cannot be read is the build's to report, naming the path
	}
	stages := airflowrt.DockerfileStages(data)
	if !slices.ContainsFunc(stages, func(st airflowrt.DockerfileStage) bool { return st.OnBuildRuntime() }) {
		return "", nil
	}
	notes, fixable, err := onBuildFixable(dir, m)
	if err != nil {
		return "", err
	}
	if fixable {
		// Init moves a file only when its final stage is the one stage on an
		// ONBUILD tag (dockerfileRewriteOf), so that is the stage to replace.
		return "", onBuildRefusal(m.Astro.Dockerfile, &stages[len(stages)-1])
	}
	if len(notes) == 0 {
		notes = []string{onBuildKeptNote(m.Astro.Dockerfile + ": builds on a runtime tag with ONBUILD steps, and astro init cannot move it onto a -base tag")}
	}
	return strings.Join(notes, "\n"), nil
}

// ErrOnBuildBase reports a build refused because the project's declared
// Dockerfile builds on an Astro Runtime tag with ONBUILD steps, which install
// requirements.txt and packages.txt rather than the manifest's lists, and
// astro init would move it onto that tag's -base image
// (CheckDockerfileOnBuild). pkg/localrt never returns it: a caller that drives
// the runtime itself, as Astro Desktop does, calls CheckDockerfileOnBuild first.
var ErrOnBuildBase = errors.New("a declared Dockerfile must not build on an Astro Runtime tag with ONBUILD steps")

// onBuildRefusal is the refusal of declared, whose final stage is on an
// ONBUILD tag astro init would move off: the lines that replace its FROM, the
// stage's flags and name kept, and what pyproject.toml has to record of the
// runtime its tag names, as init writes them.
func onBuildRefusal(declared string, final *airflowrt.DockerfileStage) error {
	rw, _ := installBaseRuntimeOf(final, "")
	from := append(append([]string{"FROM"}, final.Flags...), airflowrt.BaseArgFrom)
	if final.Alias != "" {
		from = append(from, "AS", final.Alias)
	}
	lines := "ARG " + airflowrt.BaseArg + "\n" + strings.Join(from, " ") + "\n" + airflowrt.ProjectCopy + "\n"
	var record []string
	if rw.runtime != "" {
		record = append(record, "runtime = '"+rw.runtime+"'")
	}
	if rw.registry != "" {
		record = append(record, "registry = '"+rw.registry+"'")
	}
	recorded := ""
	if len(record) > 0 {
		recorded = ", and add " + strings.Join(record, " and ") + " under [tool.astro] in " + manifest.Marker
	}
	return fmt.Errorf("%w: %s builds FROM %s, whose ONBUILD steps install requirements.txt and packages.txt "+
		"rather than what pyproject.toml lists, so the image and standalone mode would install different packages. "+
		"Run astro init, which moves the file onto the install base the Astro CLI builds from pyproject.toml and uv.lock, "+
		"or replace the FROM line with these lines, which astro init writes%s:\n\n%s\n"+
		"Build it with astro package or astro deploy, or run astro package --base or --context-dir <dir> to build it with docker",
		ErrOnBuildBase, declared, final.Ref, recorded, lines)
}

// refuseKeptDockerfileOfAnotherAirflow refuses a conversion that would keep the
// project's Dockerfile as its build beside an Airflow requirement its FROM
// disagrees with: a project CheckDockerfileAirflow then refuses on every run
// path. Only an explicit --airflow-version or an existing manifest's own pin
// can get here, since without either the pin is read from that same FROM.
//
// requirement is the Airflow requirement the manifest would carry, and source
// names where its version came from, for the message. The problem is the one
// start would report, wrapped as a *manifest.ValidationError.
func refuseKeptDockerfileOfAnotherAirflow(dir string, from1x *project1x, requirement, source string) error {
	if !declaresDockerfile(from1x) {
		return nil
	}
	m := &manifest.Manifest{
		Project: manifest.Project{Dependencies: []string{requirement}},
		Astro:   manifest.Astro{Dockerfile: fileDockerfile},
	}
	base := airflowrt.ReadDeclaredBase(filepath.Join(dir, fileDockerfile))
	p, ok := m.DockerfileRuntimeProblem(base.Ref(), base.RuntimeVersion())
	if !ok {
		return nil
	}
	return fmt.Errorf("%s disagrees with the %s this conversion keeps as the project's build, so the project would not start: %w",
		source, fileDockerfile, &manifest.ValidationError{Path: filepath.Join(dir, manifest.Marker), Problems: []manifest.Problem{p}})
}

// refuseAdoptedDockerfileOfAnotherAirflow is refuseKeptDockerfileOfAnotherAirflow
// for the adopt arm, where the version comes from --airflow-version when it is
// given, which rewrites the requirement, and otherwise from the requirement
// the existing manifest already carries, which is kept.
func refuseAdoptedDockerfileOfAnotherAirflow(dir string, from1x *project1x, flag string, deps []string) error {
	if flag != "" {
		return refuseKeptDockerfileOfAnotherAirflow(dir, from1x, airflowRequirement(flag), "--airflow-version "+flag)
	}
	for _, spec := range deps {
		if _, ok := manifest.AirflowPin(spec); ok {
			spec = strings.TrimSpace(spec)
			return refuseKeptDockerfileOfAnotherAirflow(dir, from1x, spec,
				"The Airflow requirement "+spec+" already in "+manifest.Marker)
		}
	}
	return nil
}

// ErrNoDockerfileAirflow reports a MatchAirflowToDockerfile the file cannot
// answer: no declared Dockerfile, or a FROM line naming no Astro Runtime
// version this can read.
var ErrNoDockerfileAirflow = errors.New("the declared Dockerfile names no Astro Runtime version to match")

// MatchAirflowToDockerfile moves the Airflow requirement to the Airflow the
// declared Dockerfile's FROM line names, through EditManifest, and reports it
// as SetAirflowVersionWith does: the fix for
// manifest.CodeDockerfileAirflowMismatch. Everything else in the file is left
// byte for byte, the Dockerfile included.
//
// An Airflow 3 FROM names its series, and the requirement moves to it
// ("apache-airflow==3.2.*" for runtime:3.2-4). An Airflow 2 FROM names a
// runtime version, so with opts.Catalog the requirement moves to the series
// the catalog says that build carries, and without one to the generation
// ("apache-airflow==2.*"), the most the tag says.
//
// A requirement that already agrees with the FROM is left alone and reports
// Changed false. A FROM this cannot read is ErrNoDockerfileAirflow. With
// opts.DryRun the change is reported and nothing is written.
func MatchAirflowToDockerfile(dir string, wrap func(run func() error) error, opts AirflowPinOptions) (AirflowPinChange, error) {
	var change AirflowPinChange
	err := editManifestFor(opts.DryRun, dir, wrap, func(before *manifest.Manifest, ed tomledit.Editor) error {
		path := dockerfilePath(dir, before)
		if path == "" {
			return fmt.Errorf("%w: [tool.astro] declares no dockerfile", ErrNoDockerfileAirflow)
		}
		base := airflowrt.ReadDeclaredBase(path)
		tag, ok := manifest.ParseRuntimeTag(base.RuntimeVersion())
		if !ok {
			return fmt.Errorf("%w: %s builds FROM %s", ErrNoDockerfileAirflow, before.Astro.Dockerfile, fromForMessage(base))
		}
		current := before.Airflow().Pin
		if current != "" && !before.AirflowUnclear() && tag.Agrees(current) {
			change = AirflowPinChange{Previous: current, Version: current, Dockerfile: before.Astro.Dockerfile}
			return nil
		}
		var err error
		change, err = setAirflowVersion(before, ed, versionForTag(tag, base.RuntimeVersion(), opts), opts)
		return err
	})
	if err != nil {
		return AirflowPinChange{}, err
	}
	return change, nil
}

// versionForTag is the Airflow pin a runtime tag names: its series for
// Airflow 3, and for Airflow 2 the series the catalog says the build carries,
// else the generation.
func versionForTag(tag manifest.RuntimeTag, runtime string, opts AirflowPinOptions) string {
	if tag.Series != "" {
		return tag.Series
	}
	if opts.Catalog != nil {
		if r, ok := opts.Catalog.Runtime(runtime); ok {
			if major, rest, found := strings.Cut(r.AirflowVersion, "."); found && major == tag.Major {
				minor, _, _ := strings.Cut(rest, ".")
				if minor != "" {
					return major + "." + minor
				}
			}
		}
	}
	return tag.Major
}

// fromForMessage names what a FROM line said, for an error that could not use it.
func fromForMessage(b airflowrt.DeclaredBase) string {
	if !b.Known {
		return "a base that could not be read (a build argument, or no FROM line)"
	}
	if b.Tag == "" {
		return b.Image + ", pinned by digest"
	}
	return b.Ref()
}

// catalogRuntime is catalog.Runtime for a catalog that may be nil.
func catalogRuntime(catalog *runtimeversions.Catalog, build string) (runtimeversions.Runtime, bool) {
	if catalog == nil {
		return runtimeversions.Runtime{}, false
	}
	return catalog.Runtime(build)
}
