package deploybuild

import (
	"context"

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
}

// OfferedBase is the rule a platform's deploy follows for a generated build's
// runtime base. A [tool.astro] runtime the manifest names wins. Otherwise the
// newest build offer has for the pin becomes the build, so the image carries
// a runtime the platform takes. With none offered (or offer nil), resolve
// runs with no build named and is the platform's own fallback: a series tag
// for Airflow 3, the public catalog for Airflow 2, and the refusal when that
// has nothing either.
//
// resolve gets the manifest build with Runtime filled in when an offered
// build was picked, so it can tell the two cases apart by m.Runtime.
func OfferedBase(offer RuntimeOffer, resolve imagebuild.BaseImageFunc) imagebuild.BaseImageFunc {
	return func(ctx context.Context, m imagebuild.ManifestBuild) (string, error) {
		if m.Runtime == "" && offer != nil {
			m.Runtime = offer.NewestFor(m.AirflowVersion)
		}
		return resolve(ctx, m)
	}
}
