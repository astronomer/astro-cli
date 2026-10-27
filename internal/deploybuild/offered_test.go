package deploybuild

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// offer maps a pin to its offered builds, newest first; every build it lists
// is offered.
type offer map[string][]string

func (o offer) BuildsFor(pin string) []string { return o[pin] }

// The manifest's runtime wins, then the offer's newest for the pin, and with
// neither the platform's own fallback runs with no build named.
func TestOfferedBase(t *testing.T) {
	var got []string
	resolve := func(_ context.Context, m imagebuild.ManifestBuild) (string, error) {
		got = append(got, m.Runtime)
		return "base:" + m.Runtime, nil
	}
	base := OfferedBase(offer{"2.10": {"12.6.0"}}, resolve, nil)

	for _, tt := range []struct{ pin, runtime, want string }{
		{"2.10", "12.9.0", "12.9.0"},
		{"2.10", "", "12.6.0"},
		{"2.9", "", ""},
	} {
		ref, err := base(context.Background(), imagebuild.ManifestBuild{AirflowVersion: tt.pin, Runtime: tt.runtime})
		require.NoError(t, err)
		assert.Equal(t, "base:"+tt.want, ref, tt.pin)
	}
	assert.Equal(t, []string{"12.9.0", "12.6.0", ""}, got)

	ref, err := OfferedBase(nil, resolve, nil)(context.Background(), imagebuild.ManifestBuild{AirflowVersion: "2.10"})
	require.NoError(t, err)
	assert.Equal(t, "base:", ref, "no offer is the fallback")
}

// The build the lock was made for is the base when the Deployment offers it,
// even when it offers a newer one. When it does not, the newest offered build
// is, with a warning naming both; with nothing offered for the pin, or no
// offer, the platform's own fallback runs with no build named, as without a
// lock. [tool.astro] runtime still wins over all of it, and a bare "3" pin
// resolves to no offered build at all.
func TestOfferedBasePrefersTheLocksBuild(t *testing.T) {
	resolve := func(_ context.Context, m imagebuild.ManifestBuild) (string, error) {
		return m.Runtime, nil
	}
	o := offer{"3.3": {"3.3-10", "3.3-9"}, "3": {"3.3-10"}}
	for _, tt := range []struct {
		name          string
		offer         RuntimeOffer
		build         imagebuild.ManifestBuild
		want          string
		warnMentioned []string
	}{
		{name: "offered lock build", offer: o, build: imagebuild.ManifestBuild{AirflowVersion: "3.3", LockedRuntime: "3.3-9"}, want: "3.3-9"},
		{
			name: "lock build not offered", offer: o, build: imagebuild.ManifestBuild{AirflowVersion: "3.3", LockedRuntime: "3.3-8"},
			want: "3.3-10", warnMentioned: []string{"3.3-8", "3.3-10", "does not offer", "astro init"},
		},
		{name: "nothing offered for the pin", offer: o, build: imagebuild.ManifestBuild{AirflowVersion: "3.2", LockedRuntime: "3.2-4"}, want: ""},
		{name: "no offer", offer: nil, build: imagebuild.ManifestBuild{AirflowVersion: "3.3", LockedRuntime: "3.3-8"}, want: ""},
		{name: "no lock build", offer: o, build: imagebuild.ManifestBuild{AirflowVersion: "3.3"}, want: "3.3-10"},
		{name: "runtime wins", offer: o, build: imagebuild.ManifestBuild{AirflowVersion: "3.3", Runtime: "3.3-7", LockedRuntime: "3.3-9"}, want: "3.3-7"},
		{name: "bare 3", offer: o, build: imagebuild.ManifestBuild{AirflowVersion: "3", LockedRuntime: "3.3-10"}, want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var warnings []string
			ref, err := OfferedBase(tt.offer, resolve, func(s string) { warnings = append(warnings, s) })(context.Background(), tt.build)
			require.NoError(t, err)
			assert.Equal(t, tt.want, ref)
			if len(tt.warnMentioned) == 0 {
				assert.Empty(t, warnings)
				return
			}
			require.Len(t, warnings, 1)
			for _, s := range tt.warnMentioned {
				assert.Contains(t, warnings[0], s)
			}
		})
	}
}

// The lock's build runs the recorded Python, as a local or packaged image of
// it does; a requires-python that excludes that Python moves on to the next
// offered build, with a warning.
func TestOfferedBaseRunsTheLocksBuildOnTheRecordedPython(t *testing.T) {
	var asked []imagebuild.ManifestBuild
	resolve := func(_ context.Context, m imagebuild.ManifestBuild) (string, error) {
		asked = append(asked, m)
		return m.Runtime, nil
	}
	o := offer{"3.3": {"3.3-10", "3.3-9"}}

	ref, err := OfferedBase(o, resolve, nil)(context.Background(),
		imagebuild.ManifestBuild{AirflowVersion: "3.3", LockedRuntime: "3.3-9", LockedPython: "3.12", RequiresPython: ">=3.12"})
	require.NoError(t, err)
	assert.Equal(t, "3.3-9", ref)
	assert.Equal(t, "==3.12.*", asked[0].RequiresPython)

	asked = nil
	var warnings []string
	ref, err = OfferedBase(o, resolve, func(s string) { warnings = append(warnings, s) })(context.Background(),
		imagebuild.ManifestBuild{AirflowVersion: "3.3", LockedRuntime: "3.3-9", LockedPython: "3.12", RequiresPython: ">=3.13"})
	require.NoError(t, err)
	assert.Equal(t, "3.3-10", ref, "requires-python excludes the recorded 3.12")
	assert.Equal(t, ">=3.13", asked[0].RequiresPython)
	require.NotEmpty(t, warnings)
	assert.Contains(t, warnings[0], "excludes Python 3.12")
}

// An offered build that ships no Python requires-python admits gives way to
// the next offered build that does, newest first, and the warning names the
// build skipped and why: the lock's build when it was that one, else the
// newest. With every offered build refused so, the deploy is refused; the
// platform's own fallback, which can name a build it does not offer, does not
// run.
func TestOfferedBaseStaysWithinTheOfferWhenAPythonIsNotShipped(t *testing.T) {
	ships := map[string]bool{"3.3-8": true}
	resolve := func(_ context.Context, m imagebuild.ManifestBuild) (string, error) {
		if m.Runtime == "" {
			return "fallback", nil
		}
		if !ships[m.Runtime] {
			return "", &runtimeversions.PythonNotShippedError{RequiresPython: ">=3.13", Build: m.Runtime, Pythons: []string{"3.12"}}
		}
		return m.Runtime, nil
	}
	o := offer{"3.3": {"3.3-10", "3.3-9", "3.3-8"}}
	for _, tt := range []struct {
		name          string
		build         imagebuild.ManifestBuild
		warnMentioned []string
	}{
		{"lock build", imagebuild.ManifestBuild{AirflowVersion: "3.3", LockedRuntime: "3.3-9"}, []string{"uv.lock was made for Astro Runtime 3.3-9", "admits none of the Pythons", "3.3-8", "astro init"}},
		{"newest", imagebuild.ManifestBuild{AirflowVersion: "3.3"}, []string{"builds on Astro Runtime 3.3-8, not 3.3-10"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var warnings []string
			ref, err := OfferedBase(o, resolve, func(s string) { warnings = append(warnings, s) })(context.Background(), tt.build)
			require.NoError(t, err)
			assert.Equal(t, "3.3-8", ref)
			require.Len(t, warnings, 1)
			for _, s := range tt.warnMentioned {
				assert.Contains(t, warnings[0], s)
			}
		})
	}

	ships = map[string]bool{}
	_, err := OfferedBase(o, resolve, nil)(context.Background(), imagebuild.ManifestBuild{AirflowVersion: "3.3"})
	var notShipped *runtimeversions.PythonNotShippedError
	require.True(t, errors.As(err, &notShipped), "%v", err)
	assert.ErrorContains(t, err, "none of the builds the Deployment offers for Airflow 3.3")
}

// A lock seeded for a build of a release the pin has since moved off is not
// built on, though the Deployment lists that build for its own release: only
// the pin's offered builds are candidates, by the platform's word of each
// build's Airflow.
func TestOfferedBaseSkipsALockOfAReleaseThePinNoLongerCovers(t *testing.T) {
	var warned []string
	resolve := func(_ context.Context, m imagebuild.ManifestBuild) (string, error) { return "base:" + m.Runtime, nil }
	base := OfferedBase(offer{"3.3": {"3.3-9"}, "3.2": {"3.2-5"}}, resolve, func(s string) { warned = append(warned, s) })
	ref, err := base(context.Background(), imagebuild.ManifestBuild{AirflowVersion: "3.3", LockedRuntime: "3.2-5"})
	require.NoError(t, err)
	assert.Equal(t, "base:3.3-9", ref)
	require.Len(t, warned, 1)
	assert.Contains(t, warned[0], "3.2-5")
}
