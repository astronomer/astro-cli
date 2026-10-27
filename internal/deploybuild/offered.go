package deploybuild

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
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
	// BuildsFor are the offered builds carrying an Airflow release the pin
	// covers, newest first, of those the platform offers as a base nobody
	// chose. None for a pin the platform refuses to resolve (a bare "3").
	BuildsFor(airflowPin string) []string
}

// OfferedBase is the rule a platform's deploy follows for a generated build's
// runtime base. A [tool.astro] runtime the manifest names wins. Otherwise the
// build the project's uv.lock was made for (m.LockedRuntime) when the platform
// offers it, so the image is the build the lock matches; else the newest build
// offer has for the pin, so the image carries a runtime the platform takes,
// and warn says the lock was made for another build.
//
// A build requires-python admits none of the Pythons of
// (runtimeversions.PythonNotShippedError from resolve) gives way to the next
// offered build, newest first, that ships one, and warn names the build
// skipped and why: the image stays a runtime the platform takes. With every
// offered build refused so, the deploy is refused with the first error.
//
// With none offered for the pin (or offer nil), resolve runs with no build
// named and is the platform's own fallback: a series tag for Airflow 3, the
// public catalog for Airflow 2, and the refusal when that has nothing either.
//
// resolve gets the manifest build with Runtime filled in when a build was
// picked, so it can tell the two cases apart by m.Runtime. warn may be nil.
func OfferedBase(offer RuntimeOffer, resolve imagebuild.BaseImageFunc, warn func(string)) imagebuild.BaseImageFunc {
	if warn == nil {
		warn = func(string) {}
	}
	return func(ctx context.Context, m imagebuild.ManifestBuild) (string, error) {
		if m.Runtime != "" || offer == nil {
			return resolve(ctx, m)
		}
		candidates, locked := offeredCandidates(offer, m)
		if len(candidates) == 0 {
			return resolve(ctx, m)
		}
		var first error
		var skipped string
		for _, build := range candidates {
			named := m
			named.Runtime = build
			if build == locked && m.LockedPython != "" {
				// The lock's build runs the recorded Python, as a local and a
				// packaged image of it do (imagebuild's lockedBase), so all three
				// run one Python. A requires-python that excludes it moves on to
				// the next offered build, with a warning.
				if !runtimeversions.RequiresPythonAdmits(m.RequiresPython, m.LockedPython) {
					warn(fmt.Sprintf("requires-python %s excludes Python %s, the one this project's pyproject.toml records for Astro Runtime %s, so the image builds on the newest other build the Deployment offers",
						m.RequiresPython, m.LockedPython, locked))
					continue
				}
				named.RequiresPython = "==" + m.LockedPython + ".*"
			}
			image, err := resolve(ctx, named)
			var notShipped *runtimeversions.PythonNotShippedError
			if errors.As(err, &notShipped) {
				if first == nil {
					first, skipped = err, build
				}
				continue
			}
			if err != nil {
				return "", err
			}
			switch {
			case first != nil && skipped == locked:
				warn(fmt.Sprintf("uv.lock was made for Astro Runtime %s, which the Deployment offers, but %v, so the image builds on %s, the newest offered build that ships one, and installs the lock's versions over it. "+
					"Run astro init to move the lock to %s", locked, first, build, build))
			case first != nil:
				warn(fmt.Sprintf("the image builds on Astro Runtime %s, not %s, the newest the Deployment offers: %v", build, skipped, first))
			case m.LockedRuntime != "" && build != m.LockedRuntime:
				warn(fmt.Sprintf("uv.lock was made for Astro Runtime %s, which the Deployment does not offer, so the image builds on %s, the newest build it offers, and installs the lock's versions over that runtime. "+
					"Deploy once the Deployment offers %s, or run astro init to move the lock to the newest build", m.LockedRuntime, build, m.LockedRuntime))
			}
			return image, nil
		}
		return "", fmt.Errorf("none of the builds the Deployment offers for Airflow %s ships a Python requires-python admits: %w", m.AirflowVersion, first)
	}
}

// offeredCandidates are the builds OfferedBase tries for m, in order: the
// lock's build first when the platform offers it for the pin, then the
// offered builds of the pin, newest first. locked is the lock's build when it
// leads the list. The lock's build has to be one of the pin's offered builds:
// those carry an Airflow release the pin covers, by the platform's own word
// of each build's Airflow, so a build recorded for a release the pin has since
// moved off is not built on, and a build the platform lists only on another
// channel (a preview) is not either. A bare "3" pin has none, as every other
// path refuses to resolve one.
func offeredCandidates(offer RuntimeOffer, m imagebuild.ManifestBuild) (candidates []string, locked string) {
	if strings.TrimSpace(m.AirflowVersion) == "3" {
		return nil, ""
	}
	candidates = offer.BuildsFor(m.AirflowVersion)
	if l := m.LockedRuntime; l != "" && slices.Contains(candidates, l) {
		candidates = append([]string{l}, slices.DeleteFunc(slices.Clone(candidates), func(b string) bool { return b == l })...)
		locked = l
	}
	return candidates, locked
}
