package deploybuild

import (
	"context"
	"fmt"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// RuntimeOffer is a platform's word on which Astro Runtime builds a
// Deployment accepts: Houston's runtime releases for Astro Private Cloud, the
// Deployment's options for Astro. Each platform implements it over its own
// API, since what it lists and how it names an Airflow release are its own.
//
// The public runtime catalog says what exists; an offer says what this
// Deployment will run. They differ by builds the platform has not rolled out
// yet, builds it no longer takes, and, on an air-gapped cluster, whether the
// catalog can be read at all. A generated build that starts from an offered
// build is one the deploy's runtime check will pass.
type RuntimeOffer interface {
	// NewestFor is the newest offered build carrying an Airflow release the
	// pin covers, or "" when the platform offers none for it.
	NewestFor(airflowPin string) string
	// AirflowOf is the Airflow an offered build carries, by the platform's
	// own word, or "" when the platform does not offer it.
	AirflowOf(build string) string
}

// OfferedBase is the rule a platform's deploy follows for a generated build's
// runtime base. A [tool.astro] runtime the manifest names wins. Otherwise the
// runtime version the project's uv.lock was made for (m.LockedRuntime) when
// the platform offers it, and else the newest build offer has for the pin, so
// the image carries a runtime the platform takes. With none offered (or offer
// nil), resolve runs with no build named and is the platform's own fallback: a
// series tag for Airflow 3, the public catalog for Airflow 2, and the refusal
// when that has nothing either.
//
// When the lock's version is not offered, the newest offered one stands in
// for it only if it carries the lock's Airflow release, with one warning; one
// that carries another is refused before the build
// (imagebuild.LockedRuntimeError). warn may be nil.
//
// resolve gets the manifest build with Runtime filled in when an offered
// build was picked, so it can tell the two cases apart by m.Runtime.
func OfferedBase(offer RuntimeOffer, resolve imagebuild.BaseImageFunc, warn func(string)) imagebuild.BaseImageFunc {
	return func(ctx context.Context, m imagebuild.ManifestBuild) (string, error) {
		if m.Runtime == "" && offer != nil {
			build, err := offeredRuntime(offer, m, warn)
			if err != nil {
				return "", err
			}
			m.Runtime = build
		}
		return resolve(ctx, m)
	}
}

// offeredRuntime is the offered build OfferedBase names for m, or "" for none.
func offeredRuntime(offer RuntimeOffer, m imagebuild.ManifestBuild, warn func(string)) (string, error) {
	newest := offer.NewestFor(m.AirflowVersion)
	locked := m.LockedRuntime
	switch {
	case locked == "" || m.LockedAirflow == "" || !manifest.PinCovers(m.AirflowVersion, m.LockedAirflow):
		return newest, nil
	case offer.AirflowOf(locked) != "":
		return locked, nil
	case newest == "":
		return "", nil
	}
	if af := offer.AirflowOf(newest); !imagebuild.SameAirflowRelease(m.LockedAirflow, af) {
		return "", &imagebuild.LockedRuntimeError{
			Locked: locked, LockedAirflow: m.LockedAirflow, Why: "the Deployment does not offer it",
			Base: newest, BaseAirflow: af,
		}
	}
	if warn != nil {
		warn(fmt.Sprintf("uv.lock was made for Astro Runtime %s, which the Deployment does not offer, so the image builds on %s, which has the same Airflow", locked, newest))
	}
	return newest, nil
}
