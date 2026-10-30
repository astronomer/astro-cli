package imageinstall

import (
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// AirflowMismatchError is a build refused before it starts because uv.lock
// was made for another Airflow release than the image's base runs: the lock's
// versions were resolved against that Airflow, and installed over another one
// they are untested at best. Every build path refuses it alike, a generated
// build's and a project's own Dockerfile's, --context-dir included, since each
// resolves the lists here first.
type AirflowMismatchError struct {
	// Locked is the Airflow uv.lock pins, and Image the one the base runs.
	Locked, Image string
	// Recorded is the build pyproject.toml records for the lock, or "".
	Recorded string
	// Base is the image the build starts FROM.
	Base string
	// Dockerfile is the project's own Dockerfile, relative to the project,
	// whose FROM names Base, or "" for a generated build.
	Dockerfile string
}

// Error names both Airflows and both ways out: build on a runtime with the
// lock's Airflow (for a declared Dockerfile, its FROM line), or move the
// project to the image's Airflow with astro init. Either can be the one
// meant: after `astro local upgrade airflow` it is the Dockerfile that is
// behind, after an edited FROM it is the lock.
func (e *AirflowMismatchError) Error() string {
	madeFor, withLocked := "", "Airflow "+release(e.Locked)
	if e.Recorded != "" {
		madeFor = " (Astro Runtime " + e.Recorded + ")"
		withLocked += ", such as " + e.Recorded
	}
	build := "build on a runtime with " + withLocked
	if e.Dockerfile != "" {
		build = "change the FROM line of " + e.Dockerfile + " to a runtime with " + withLocked
	}
	return fmt.Sprintf("uv.lock was made for Airflow %s%s, and the image builds on %s, which has Airflow %s, "+
		"so the lock's versions would install over an Airflow they were not resolved for. "+
		"To keep the lock's Airflow, %s; to move to the image's, run astro init, which relocks uv.lock",
		e.Locked, madeFor, baseName(e.Base), e.Image, build)
}

// airflowMismatch is the refusal of a build whose lock, pylock (what `uv
// export --format pylock.toml` printed), pins another Airflow release than the
// image has: the freeze's apache-airflow line, else the catalog's Airflow for
// build. nil when they agree, when the lock pins no single Airflow, or when the
// image's is not known. A release is compared without its local version, so
// 3.3.2+astro.2 and 3.3.2+astro.3 are one Airflow, and PyPI's 3.3.2 too.
//
// recorded is the build pyproject.toml records for the lock (manifest's
// RuntimeBuild), named in the message when there is one; base is the image's
// runtime, and dockerfile the project's own Dockerfile that names it, or "",
// for the message.
func airflowMismatch(pylockData []byte, freeze []string, catalog *runtimeversions.Catalog, build, recorded, base, dockerfile string) error {
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
	if image == "" || release(image) == release(locked) {
		return nil
	}
	return &AirflowMismatchError{Locked: locked, Image: image, Recorded: recorded, Base: base, Dockerfile: dockerfile}
}

// lockedAirflow is the one apache-airflow version a lock's export pins, or ""
// for none, or for more than one (a marker splits it), which no one image
// can be held to.
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

// release is a version without its local part, lowercased: 3.3.2+astro.2 is
// release 3.3.2.
func release(v string) string {
	r, _, _ := strings.Cut(strings.TrimSpace(v), "+")
	return strings.ToLower(r)
}
