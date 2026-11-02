package imagebuild

import (
	"context"
	"fmt"
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// lockedAirflow is the Airflow the +astro pin in constraints holds
// apache-airflow to ("3.3.2+astro.1"), the Airflow of the runtime version
// pyproject.toml records for the lock, or "" when there is none.
func lockedAirflow(constraints []string) string {
	for _, c := range constraints {
		if !manifest.AstroPin(c) || manifest.DistName(c) != "apache-airflow" {
			continue
		}
		if _, version, ok := strings.Cut(c, "=="); ok {
			v, _, _ := strings.Cut(version, ";")
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// SameAirflowRelease reports whether two Airflow versions are one release,
// local versions aside: 3.1.3+astro.2, 3.1.3+astro.3 and PyPI's 3.1.3 are one.
// An unknown version ("") agrees with anything, so nothing is refused on a
// version that was not read.
func SameAirflowRelease(a, b string) bool {
	return a == "" || b == "" || AirflowRelease(a) == AirflowRelease(b)
}

// AirflowRelease is an Airflow version without its local part, lowercased:
// 3.3.2+astro.2 is release 3.3.2.
func AirflowRelease(v string) string {
	r, _, _ := strings.Cut(strings.TrimSpace(v), "+")
	return strings.ToLower(r)
}

// LockedRuntimeError refuses a build, before it starts, whose base would carry
// another Airflow release than the one the project's uv.lock was made for: the
// lock's versions were resolved against that Airflow.
type LockedRuntimeError struct {
	// Locked is the runtime version the lock was made for, and LockedAirflow
	// its Airflow.
	Locked, LockedAirflow string
	// Why says why the image does not build on Locked, as a clause.
	Why string
	// Base is the runtime version the image would build on instead, and
	// BaseAirflow its Airflow.
	Base, BaseAirflow string
}

func (e *LockedRuntimeError) Error() string {
	return fmt.Sprintf("uv.lock was made for Astro Runtime %s (Airflow %s), and %s. The image would build on %s, which has Airflow %s. "+
		"Run astro init to move the project to %s, or deploy once %s is available",
		e.Locked, AirflowRelease(e.LockedAirflow), e.Why, e.Base, AirflowRelease(e.BaseAirflow), e.Base, e.Locked)
}

// lockedBase is base building on the runtime version the project's lock was
// made for (ManifestBuild.LockedRuntime) when the manifest names none and the
// Airflow requirement still covers the lock's Airflow, so the image runs what
// uv.lock was resolved against. Otherwise, or when that version cannot be
// resolved, base's own rule picks the version; a lock that then disagrees
// with the image's Airflow is refused before the build (imageinstall.Fill).
func lockedBase(base BaseImageFunc) BaseImageFunc {
	return func(ctx context.Context, m ManifestBuild) (string, error) {
		if m.Runtime != "" || m.LockedRuntime == "" || m.LockedAirflow == "" || !manifest.PinCovers(m.AirflowVersion, m.LockedAirflow) {
			return base(ctx, m)
		}
		locked := m
		locked.Runtime = m.LockedRuntime
		if image, err := base(ctx, locked); err == nil {
			return image, nil
		}
		return base(ctx, m)
	}
}
