package deploybuild

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/mod/semver"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// Plan is the Astro Runtime a deploy's image will carry, as far as the
// project says before anything is built, and the change that moves it. The
// zero Plan plans no version and names no fix.
type Plan struct {
	// version is an exact runtime ("3.3-8"), a series whose moving tag the
	// build resolves ("3.2", with series set), or "" when only the built
	// image can say.
	version string
	series  bool
	// raise names the change that gives the image runtime minimum or newer.
	raise func(minimum string) string
}

// PlanRuntime reads the runtime the image will carry from what decides it: the
// runtime base of a generated build, or the final FROM of a declared
// Dockerfile. imageName is a prebuilt image (--image-name), "" for one built
// from the manifest build; req is the image the manifest builds
// (imagebuild.ForManifest's). A prebuilt image, an Airflow 2 base and a FROM
// this cannot read plan no version.
//
// A runtime build and a FROM have to agree with the apache-airflow pin's
// series, so a fix that moves either to another series names the pin too.
func PlanRuntime(imageName string, req *imagebuild.Request, build *imagebuild.ManifestBuild) Plan {
	switch {
	case imageName != "":
		return Plan{raise: func(minimum string) string {
			return fmt.Sprintf("rebuild %s FROM Astro Runtime %s or newer", imageName, minimum)
		}}
	case req.FromDeclaredDockerfile():
		v := airflowrt.ReadDeclaredBase(req.Dockerfile).RuntimeVersion()
		dockerfile := build.Dockerfile
		p := Plan{raise: func(minimum string) string {
			fix := fmt.Sprintf("change the FROM line in %s to Astro Runtime %s or newer", dockerfile, minimum)
			if s := runtimeSeries(minimum); s != "" && s != runtimeSeries(v) {
				fix += fmt.Sprintf(", and pin apache-airflow to %s in pyproject.toml", s)
			}
			return fix
		}}
		if tag, ok := manifest.ParseRuntimeTag(v); ok && tag.Series != "" {
			p.version, p.series = v, !tag.Build
		}
		return p
	case build.Runtime != "":
		runtime := build.Runtime
		return Plan{version: runtime, raise: func(minimum string) string {
			if s := runtimeSeries(minimum); s != "" && s != runtimeSeries(runtime) {
				return fmt.Sprintf("pin apache-airflow to %s and set [tool.astro] runtime to %s or newer in pyproject.toml", s, minimum)
			}
			return fmt.Sprintf("set [tool.astro] runtime to %s or newer in pyproject.toml", minimum)
		}}
	default:
		_, ref, _ := strings.Cut(req.BaseImage, ":")
		tag, _ := airflowrt.ParseRuntimeTagPython(ref)
		parsed, _ := manifest.ParseRuntimeTag(tag)
		return Plan{version: tag, series: !parsed.Build, raise: func(minimum string) string {
			s := runtimeSeries(minimum)
			if s == runtimeSeries(tag) {
				// The pin is already the deployment's series; only the build
				// it resolved to (its moving tag, or the exact build a
				// requires-python Python flavor names) is older.
				return fmt.Sprintf("set [tool.astro] runtime to %s or newer in pyproject.toml", minimum)
			}
			return fmt.Sprintf("pin apache-airflow to %s or newer in pyproject.toml", cmp.Or(s, minimum))
		}}
	}
}

// Check runs CheckBuilt's rules on the runtime the image will carry, before it
// is built, against the Deployment's current runtime and the runtimes it
// accepts. An exact version takes the rules as they are. A series refuses only
// what no build of it can pass: a series older than the deployment's, or one
// the deployment offers no build of. Its patch is left to the check on the
// built image.
func (p *Plan) Check(currentVersion string, allowed []string) error {
	if currentVersion == "" || p.version == "" {
		return nil
	}
	if !p.series {
		return checkRuntimeVersion(currentVersion, p.version, allowed, p.raise)
	}
	if current := runtimeSeries(currentVersion); current != "" && semver.Compare("v"+p.version, "v"+current) < 0 {
		return downgradeError(p.version, currentVersion, p.raise)
	}
	if !slices.ContainsFunc(allowed, func(v string) bool { return runtimeSeries(v) == p.version }) {
		return fmt.Errorf("cannot deploy unsupported Astro Runtime %s; supported versions: %s", p.version, strings.Join(allowed, ", "))
	}
	return checkAirflow3Floor(currentVersion, p.version+"-0")
}

// CheckBuilt holds the runtime the built image carries (its label) to the
// Deployment's current runtime and the runtimes it accepts, naming the plan's
// fix for a downgrade.
func (p *Plan) CheckBuilt(currentVersion, built string, allowed []string) error {
	return checkRuntimeVersion(currentVersion, built, allowed, p.raise)
}

// runtimeSeries is the Airflow series an Airflow 3 runtime version belongs to:
// "3.3" for "3.3-8" and for "3.3". It is "" for an Airflow 2 runtime, whose
// version does not show its Airflow series.
func runtimeSeries(version string) string {
	tag, _ := manifest.ParseRuntimeTag(version)
	return tag.Series
}

func downgradeError(tag, currentVersion string, raise func(string) string) error {
	msg := fmt.Sprintf("cannot deploy Astro Runtime %s: it is a downgrade from the deployment's current %s", tag, currentVersion)
	if raise != nil {
		msg += "; to deploy, " + raise(currentVersion)
	}
	return errors.New(msg)
}

// checkRuntimeVersion is Astro CLI 1.x's runtime check without the prints: it returns
// a descriptive error instead of printing the reason (and leaving the caller to
// exit), so the manifest path stays print-free below cmd. The rules are
// identical: no downgrade (an Airflow 3 runtime is newer than every Airflow 2
// one, so 3 to 2 is one), the version must be one the deployment allows, and
// an Airflow 2-to-3 jump needs the deployment at Runtime 12.0.0 or higher.
// raise, when set, names the fix for a downgrade.
func checkRuntimeVersion(currentVersion, tag string, allowed []string, raise func(string) string) error {
	// Old deployments carry no runtime version; nothing to check against.
	if currentVersion == "" {
		return nil
	}
	if airflowversions.CompareRuntimeVersions(tag, currentVersion) < 0 {
		return downgradeError(tag, currentVersion, raise)
	}
	supported := false
	for _, v := range allowed {
		if airflowversions.CompareRuntimeVersions(tag, v) == 0 {
			supported = true
			break
		}
	}
	if !supported {
		return fmt.Errorf("cannot deploy unsupported Astro Runtime %s; supported versions: %s", tag, strings.Join(allowed, ", "))
	}
	return checkAirflow3Floor(currentVersion, tag)
}

func checkAirflow3Floor(currentVersion, tag string) error {
	if airflowversions.AirflowMajorVersionForRuntimeVersion(currentVersion) == "2" &&
		airflowversions.AirflowMajorVersionForRuntimeVersion(tag) == "3" &&
		airflowversions.CompareRuntimeVersions(currentVersion, "12.0.0") < 0 {
		return fmt.Errorf("cannot upgrade from Airflow 2 to Airflow 3 unless the deployment is at Astro Runtime 12.0.0 or higher (currently %s)", currentVersion)
	}
	return nil
}
