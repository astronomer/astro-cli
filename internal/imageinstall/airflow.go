package imageinstall

import (
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// AirflowMismatchError refuses a build before it starts because uv.lock was
// made for another Airflow release than the image's runtime version has: the
// lock's versions were resolved against that Airflow. Every path that installs
// from the lock refuses it alike, --context-dir included, since each resolves
// its lists here first.
type AirflowMismatchError struct {
	// Locked is the Airflow uv.lock pins, and Recorded the runtime version
	// pyproject.toml records for the lock, or "".
	Locked, Recorded string
	// Image is the Airflow the image's runtime version has, and Runtime that
	// version (or the base image, when the version cannot be told).
	Image, Runtime string
}

func (e *AirflowMismatchError) Error() string {
	keep := "pick a runtime version with Airflow " + imagebuild.AirflowRelease(e.Locked) + " (astro local upgrade runtime <version>)"
	madeFor := ""
	if e.Recorded != "" {
		keep = "run astro local upgrade runtime " + e.Recorded
		madeFor = " (Astro Runtime " + e.Recorded + ")"
	}
	return fmt.Sprintf("uv.lock was made for Airflow %s%s, but the image builds on %s, which has Airflow %s. "+
		"To keep the lock's Airflow, %s. To use the image's, run astro init, which relocks uv.lock",
		imagebuild.AirflowRelease(e.Locked), madeFor, e.Runtime, imagebuild.AirflowRelease(e.Image), keep)
}

// airflowMismatch is the refusal of a build whose lock export (pylockData)
// pins another Airflow release than the image has: its freeze's apache-airflow
// line, else the catalog's Airflow for build. nil when they agree, when the
// lock pins no single Airflow, or when the image's is not known.
func airflowMismatch(pylockData, pyproject []byte, freeze []string, catalog *runtimeversions.Catalog, build, base string) error {
	locked := lockedAirflow(pylockData)
	if locked == "" {
		return nil
	}
	image := freezeVersion(freeze, airflowDist)
	if image == "" && catalog != nil && build != "" {
		if r, ok := catalog.Runtime(build); ok {
			image = r.AirflowVersion
		}
	}
	if imagebuild.SameAirflowRelease(locked, image) {
		return nil
	}
	e := &AirflowMismatchError{Locked: locked, Image: image, Runtime: "Astro Runtime " + build}
	if build == "" {
		e.Runtime = baseName(base)
	}
	if m, err := manifest.Parse(pyproject); err == nil {
		e.Recorded = m.UV.RuntimeBuild
	}
	return e
}

// lockedAirflow is the one apache-airflow version a lock's export pins, or ""
// for none, or for more than one (a marker splits it), which no one image can
// be held to.
func lockedAirflow(pylockData []byte) string {
	var lock pylock
	if toml.Unmarshal(pylockData, &lock) != nil {
		return ""
	}
	version := ""
	for i := range lock.Packages {
		p := &lock.Packages[i]
		if manifest.DistName(p.Name) != airflowDist || p.Version == "" {
			continue
		}
		if version != "" && version != p.Version {
			return ""
		}
		version = p.Version
	}
	return version
}

// freezeVersion is the version a freeze's name==version lines give dist, or "".
func freezeVersion(freeze []string, dist string) string {
	for _, line := range freeze {
		if pinName(line) != dist {
			continue
		}
		if _, version, ok := strings.Cut(line, "=="); ok {
			v, _, _ := strings.Cut(version, "#")
			return strings.TrimSpace(v)
		}
	}
	return ""
}
