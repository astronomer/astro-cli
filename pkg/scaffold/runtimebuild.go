package scaffold

import (
	"errors"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// ErrNoRuntimeBuild reports a project whose image is no Astro Runtime build
// the catalog has: a declared Dockerfile on another image, or an Airflow pin
// no published build carries.
var ErrNoRuntimeBuild = errors.New("this project builds on no Astro Runtime the catalog lists")

// ErrRuntimeUnknown reports a project whose build, or whose image's Python,
// cannot be told here: no catalog, or a Dockerfile whose FROM is built from a
// build argument or pinned by digest.
var ErrRuntimeUnknown = errors.New("which Astro Runtime this project builds on cannot be told")

// RuntimeBuildFor is the Astro Runtime build a project's image is built from,
// and the Python that image runs. It is named in one place, by the first of
// these that applies:
//
//   - a declared Dockerfile's FROM, when that line picks the image: its exact
//     build, or for a floating tag ("3.3", "12") the newest published build
//     the tag serves; the Python its tag names, or else the build's own. A
//     Dockerfile built on the install base (FROM ${ASTRO_BASE}) builds on
//     what the rest of this list picks, as a generated image does;
//   - [tool.astro] runtime;
//   - the Airflow pin: the newest published build carrying a release the pin
//     covers, an exact patch when the pin names one. That is the release
//     runtimeversions.LookupAstroBuild holds Airflow to.
//
// For the last two the Python is the one the project's standalone venv and
// its generated image run (runtimeversions.ProjectPython). Where that has no
// answer (an Airflow 2 pin, no requires-python), it is the build's own
// (BuildPython). python is "" when the build is known and its Python is not:
// an Airflow 3 build without a catalog.
//
// ErrNoRuntimeBuild means the project has no build; ErrRuntimeUnknown means
// it cannot be told, as without a catalog (c nil) and no build named.
func RuntimeBuildFor(dir string, m *manifest.Manifest, c *runtimeversions.Catalog) (build, python string, err error) {
	if declaredOwnBase(dir, m) {
		return dockerfileRuntimeBuild(airflowrt.ReadDeclaredBase(dockerfilePath(dir, m)), c)
	}
	airflow := m.Airflow()
	build = strings.TrimSpace(airflow.Runtime)
	if build == "" {
		if c == nil {
			return "", "", ErrRuntimeUnknown
		}
		var ok bool
		if build, ok = c.NewestPublishedRuntimeFor(airflow.Pin); !ok || airflow.Pin == "" {
			return "", "", ErrNoRuntimeBuild
		}
	}
	if c != nil {
		// The Python of the build resolved here, not of the series' newest:
		// for a patch pin they can ship different Pythons.
		python, _, err := runtimeversions.ProjectPython(airflow.Pin, build, m.Project.RequiresPython, func() *runtimeversions.Catalog { return c })
		if err == nil && python != "" {
			return build, python, nil
		}
	}
	return build, BuildPython(build, m.Project.RequiresPython, c), nil
}

// DockerfileBuild is the one Astro Runtime build a declared Dockerfile's final
// stage is FROM ("12.6.0", "3.3-11"), read offline from the file, or "" when
// the project declares none or its FROM names no single build (a floating or
// flavor tag, a build argument, a non-runtime base). It is what the
// Astronomer-build lookup asks about instead of the newest build of the pin,
// so the lockfile's Airflow is the image's.
func DockerfileBuild(dir string, m *manifest.Manifest) string {
	path := dockerfilePath(dir, m)
	if path == "" {
		return ""
	}
	v := airflowrt.ReadDeclaredBase(path).RuntimeVersion()
	if t, ok := manifest.ParseRuntimeTag(v); ok && t.Build {
		return v
	}
	return ""
}

// BuildPython is the Python a generated image of build runs for a project's
// requires-python (runtimeversions.Runtime.ImagePython): the build's default
// when requires-python admits it, else the newest it ships that it admits. An
// Airflow 2 build lists no Python, and runs its own. "" when it cannot be
// told, or the build ships none requires-python admits.
func BuildPython(build, requiresPython string, c *runtimeversions.Catalog) string {
	if c != nil {
		if r, ok := c.Runtime(build); ok {
			if python, decided := r.ImagePython(requiresPython); decided {
				return python
			}
		}
	}
	return imagePython(build, "", c)
}

// dockerfileRuntimeBuild is RuntimeBuildFor for a declared Dockerfile's base.
func dockerfileRuntimeBuild(base airflowrt.DeclaredBase, c *runtimeversions.Catalog) (build, python string, err error) {
	if !base.Known || base.Tag == "" {
		return "", "", ErrRuntimeUnknown
	}
	version := base.RuntimeVersion()
	if version == "" {
		return "", "", ErrNoRuntimeBuild
	}
	tag, ok := manifest.ParseRuntimeTag(version)
	if !ok {
		return "", "", ErrRuntimeUnknown
	}
	build = version
	if !tag.Build {
		if c == nil {
			return "", "", ErrRuntimeUnknown
		}
		if build, ok = c.NewestPublishedRuntimeTagged(version); !ok {
			return "", "", ErrNoRuntimeBuild
		}
	}
	_, tagPython := airflowrt.ParseRuntimeTagPython(base.Tag)
	return build, imagePython(build, tagPython, c), nil
}

// imagePython is the Python the image of build runs: tagPython when its tag
// named one, else the build's own (runtimeversions.ImagePythonOf), which is ""
// for an Airflow 3 build without a catalog.
func imagePython(build, tagPython string, c *runtimeversions.Catalog) string {
	if tagPython != "" {
		return tagPython
	}
	return runtimeversions.ImagePythonOf(c, build)
}
