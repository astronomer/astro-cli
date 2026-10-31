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
	// AirflowOf is the Airflow an offered build carries, by the platform's
	// own word, or "" when it does not say.
	AirflowOf(build string) string
}

// OfferedBase is the rule a platform's deploy follows for a generated build's
// runtime base. A [tool.astro] runtime the manifest names wins. Otherwise the
// build the project's uv.lock was made for (m.LockedRuntime) when the platform
// offers it, so the image is the build the lock matches; else the newest build
// offer has for the pin, so the image carries a runtime the platform takes.
//
// A build requires-python admits none of the Pythons of
// (runtimeversions.PythonNotShippedError from resolve) gives way to the next
// offered build, newest first, that ships one: the image stays a runtime the
// platform takes. With every offered build refused so, the deploy is refused
// with the first error.
//
// When the image does not start FROM the lock's build (not offered, or its
// Python ruled out), warn says why and that the lock's versions install over
// the build it does start FROM, but only when that build carries the lock's
// Airflow release (m.LockedAirflow, by offer.AirflowOf). One that carries
// another is refused before the build, in one message
// (imagebuild.LockedBuildError), as the lock's versions were not resolved for
// it.
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
		return offeredBuild(ctx, offer, resolve, m, candidates, locked, warn)
	}
}

// offeredBuild is OfferedBase's pick among candidates, the offered builds in
// the order offeredCandidates gives them, locked the lock's build when it
// leads them: the first that resolves, past those whose Python is ruled out.
func offeredBuild(ctx context.Context, offer RuntimeOffer, resolve imagebuild.BaseImageFunc, m imagebuild.ManifestBuild,
	candidates []string, locked string, warn func(string),
) (string, error) {
	// lockedWhy says why the image does not start FROM the lock's build, as
	// a clause, once that is known.
	lockedWhy := ""
	if m.LockedRuntime != "" && locked == "" {
		lockedWhy = "the Deployment does not offer it"
	}
	var first error
	var skipped string
	for _, build := range candidates {
		named, why := candidateBuild(m, build, locked)
		if why != "" {
			lockedWhy = why
			continue
		}
		image, err := resolve(ctx, named)
		if notShipped := (*runtimeversions.PythonNotShippedError)(nil); errors.As(err, &notShipped) {
			if first == nil {
				first, skipped = err, build
			}
			if build == locked {
				lockedWhy = err.Error()
			}
			continue
		}
		switch {
		case err != nil:
			return "", err
		case m.LockedRuntime != "" && build != m.LockedRuntime:
			if err := offeredFallback(offer, m, build, lockedWhy, locked != "", warn); err != nil {
				return "", err
			}
		case first != nil:
			warn(fmt.Sprintf("the image builds on Astro Runtime %s, not %s, the newest the Deployment offers: %v", build, skipped, first))
		}
		return image, nil
	}
	return "", fmt.Errorf("none of the builds the Deployment offers for Airflow %s ships a Python requires-python admits: %w", m.AirflowVersion, first)
}

// offeredFallback says, through warn, that a deploy of m builds on build, an
// offered build other than the lock's, for why, and installs the lock's
// versions over it; or refuses it (imagebuild.LockedBuildError) when build
// carries another Airflow release than the lock. offered reports the lock's
// build among the Deployment's offer.
func offeredFallback(offer RuntimeOffer, m imagebuild.ManifestBuild, build, why string, offered bool, warn func(string)) error {
	fix := "Run astro init to move the project to " + build + ", which relocks uv.lock"
	if !offered {
		fix = "Deploy once the Deployment offers " + m.LockedRuntime + ", or run astro init to move the project to " + build + ", which relocks uv.lock"
	}
	if af := offer.AirflowOf(build); !imagebuild.SameAirflowRelease(m.LockedAirflow, af) {
		return &imagebuild.LockedBuildError{
			Locked: m.LockedRuntime, LockedAirflow: m.LockedAirflow, Why: why,
			Fallback: build, FallbackAirflow: af, Fix: fix,
		}
	}
	warn(fmt.Sprintf("uv.lock was made for Astro Runtime %s, and %s, so the image builds on %s, which the Deployment offers, and installs the lock's versions over that runtime. %s",
		m.LockedRuntime, why, build, fix))
	return nil
}

// candidateBuild is m with build named, for OfferedBase to resolve. The lock's
// build runs the recorded Python, as a local and a packaged image of it do
// (imagebuild's lockedBase), so all three run one Python. A requires-python
// that excludes that Python skips the lock's build, saying why, and the next
// offered build is tried.
func candidateBuild(m imagebuild.ManifestBuild, build, locked string) (named imagebuild.ManifestBuild, skip string) {
	named = m
	named.Runtime = build
	if build != locked || m.LockedPython == "" {
		return named, ""
	}
	if !runtimeversions.RequiresPythonAdmits(m.RequiresPython, m.LockedPython) {
		return named, fmt.Sprintf("requires-python %s excludes Python %s, the one this project's pyproject.toml records for it", m.RequiresPython, m.LockedPython)
	}
	named.RequiresPython = "==" + m.LockedPython + ".*"
	return named, ""
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
