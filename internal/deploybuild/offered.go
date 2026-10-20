package deploybuild

import (
	"context"
	"fmt"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
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
	// Offers reports whether the platform offers build ("3.3-9", "12.6.0").
	Offers(build string) bool
}

// OfferedBase is the rule a platform's deploy follows for a generated build's
// runtime base. A [tool.astro] runtime the manifest names wins. Otherwise the
// build the project's uv.lock was made for (m.LockedRuntime) when the platform
// offers it, so the image is the build the lock matches. Else the newest build
// offer has for the pin, so the image carries a runtime the platform takes,
// and warn says the lock was made for another build. With none offered for the
// pin (or offer nil), resolve runs with no build named and is the platform's
// own fallback: a series tag for Airflow 3, the public catalog for Airflow 2,
// and the refusal when that has nothing either. The lock's build is a base
// only when the platform offers it.
//
// resolve gets the manifest build with Runtime filled in when a build was
// picked, so it can tell the two cases apart by m.Runtime. warn may be nil.
func OfferedBase(offer RuntimeOffer, resolve imagebuild.BaseImageFunc, warn func(string)) imagebuild.BaseImageFunc {
	return func(ctx context.Context, m imagebuild.ManifestBuild) (string, error) {
		if m.Runtime == "" {
			m.Runtime = offeredBuild(offer, m, warn)
		}
		return resolve(ctx, m)
	}
}

// offeredBuild is the build OfferedBase names for a manifest that names none:
// the lock's build when it is offered, else the newest offered for the pin
// (warning when the lock was made for another), else "".
func offeredBuild(offer RuntimeOffer, m imagebuild.ManifestBuild, warn func(string)) string {
	if offer == nil {
		return ""
	}
	locked := m.LockedRuntime
	if locked != "" && offer.Offers(locked) {
		return locked
	}
	newest := offer.NewestFor(m.AirflowVersion)
	if newest != "" && locked != "" && warn != nil {
		warn(fmt.Sprintf("uv.lock was made for Astro Runtime %s, which the Deployment does not offer, so the image builds on %s, the newest build it offers, and installs the lock's versions over that runtime. "+
			"Deploy once the Deployment offers %s, or set [tool.astro] runtime = '%s' and run astro local start to relock for it", locked, newest, locked, newest))
	}
	return newest
}
