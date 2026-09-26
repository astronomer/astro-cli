package scaffold

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
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
	path := dockerfilePath(dir, m)
	if path == "" {
		return nil
	}
	base := airflowrt.ReadDeclaredBase(path)
	p, ok := m.DockerfileRuntimeProblem(base.Ref(), base.RuntimeVersion())
	if !ok {
		return nil
	}
	return &manifest.ValidationError{Path: filepath.Join(dir, manifest.Marker), Problems: []manifest.Problem{p}}
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
func refuseKeptDockerfileOfAnotherAirflow(dir string, v1 *v1Project, requirement, source string) error {
	if !declaresDockerfile(v1) {
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
func refuseAdoptedDockerfileOfAnotherAirflow(dir string, v1 *v1Project, flag string, deps []string) error {
	if flag != "" {
		return refuseKeptDockerfileOfAnotherAirflow(dir, v1, airflowRequirement(flag), "--airflow-version "+flag)
	}
	for _, spec := range deps {
		if _, ok := manifest.AirflowPin(spec); ok {
			spec = strings.TrimSpace(spec)
			return refuseKeptDockerfileOfAnotherAirflow(dir, v1, spec,
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
// Changed false. A FROM this cannot read is ErrNoDockerfileAirflow.
func MatchAirflowToDockerfile(dir string, wrap func(run func() error) error, opts AirflowPinOptions) (AirflowPinChange, error) {
	var change AirflowPinChange
	err := EditManifest(dir, wrap, func(before *manifest.Manifest, ed tomledit.Editor) error {
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
