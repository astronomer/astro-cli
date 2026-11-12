package scaffold

import (
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// airflow3RuntimeImage is the repository a generated Airflow 3 build pulls its
// base from (pkg/imagebuild's RuntimeImageRepo).
const airflow3RuntimeImage = "astrocrpublic.azurecr.io/runtime"

// recordedRuntime is what pyproject.toml records of a runtime tag: the one
// runtime version it names ("3.3-8", "12.6.0"), or "" for a series tag, and
// the Python a -python-X.Y flavor names, or "".
type recordedRuntime struct {
	runtime, python string
}

// recordRuntime reads what pyproject.toml has to hold of an Astro Runtime
// image, image:tag, for a generated build to build on the same runtime. why
// says what stops it: a mirror, which a generated build does not pull from; a
// flavor pyproject.toml has no spelling for; or a tag that names no runtime
// version it can read. A floating Airflow 2 tag, or a runtime older than 9,
// records nothing and stops nothing: the pin alone stands for it, with the
// note the tag's reader gives.
func recordRuntime(image, tag string) (rec recordedRuntime, why string) {
	raw := strings.TrimPrefix(tag, "v")
	noPython := raw
	if loc := airflowrt.RuntimePythonRe.FindStringIndex(raw); loc != nil {
		noPython = raw[:loc[0]]
	}
	v, python := airflowrt.ParseRuntimeTagPython(raw)
	t, ok := manifest.ParseRuntimeTag(v)
	switch {
	case !ok || strings.HasSuffix(raw, "-base"):
		return rec, "astro does not read a runtime version from its tag " + tag
	case noPython != v:
		return rec, "its tag " + tag + " names the " + strings.TrimPrefix(noPython, v+"-") + " flavor, which pyproject.toml has no way to pick"
	}
	// Astronomer publishes Airflow 3 runtimes to both repositories, Airflow 2
	// ones to quay.io alone, where a generated build pulls them from.
	official := image == airflow2RuntimeImage || (t.Major != "2" && image == airflow3RuntimeImage)
	switch {
	case !official:
		return rec, "its base " + image + " is not Astronomer's repository for Airflow " + t.Major + ", which a project without a Dockerfile builds from"
	case t.Major == "2" && (!t.Build || runtimeMajor(v) < minAirflow2Runtime):
		return rec, ""
	}
	if t.Build {
		rec.runtime = v
	}
	rec.python = python
	return rec, ""
}
