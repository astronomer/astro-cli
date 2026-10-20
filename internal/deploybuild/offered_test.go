package deploybuild

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
)

// offer maps a pin to its newest offered build; every build it maps to, and
// those in also, are offered.
type offer struct {
	newest map[string]string
	also   []string
}

func (o offer) NewestFor(pin string) string { return o.newest[pin] }

func (o offer) Offers(build string) bool {
	for _, b := range o.newest {
		if b == build {
			return true
		}
	}
	for _, b := range o.also {
		if b == build {
			return true
		}
	}
	return false
}

// The manifest's runtime wins, then the offer's newest for the pin, and with
// neither the platform's own fallback runs with no build named.
func TestOfferedBase(t *testing.T) {
	var got []string
	resolve := func(_ context.Context, m imagebuild.ManifestBuild) (string, error) {
		got = append(got, m.Runtime)
		return "base:" + m.Runtime, nil
	}
	base := OfferedBase(offer{newest: map[string]string{"2.10": "12.6.0"}}, resolve, nil)

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
// offer, the lock's build. [tool.astro] runtime still wins over all of it.
func TestOfferedBasePrefersTheLocksBuild(t *testing.T) {
	resolve := func(_ context.Context, m imagebuild.ManifestBuild) (string, error) {
		return m.Runtime, nil
	}
	o := offer{newest: map[string]string{"3.3": "3.3-10"}, also: []string{"3.3-9"}}
	for _, tt := range []struct {
		name          string
		offer         RuntimeOffer
		build         imagebuild.ManifestBuild
		want          string
		wantWarn      bool
		warnMentioned []string
	}{
		{name: "offered lock build", offer: o, build: imagebuild.ManifestBuild{AirflowVersion: "3.3", LockedRuntime: "3.3-9"}, want: "3.3-9"},
		{
			name: "lock build not offered", offer: o, build: imagebuild.ManifestBuild{AirflowVersion: "3.3", LockedRuntime: "3.3-8"},
			want: "3.3-10", wantWarn: true, warnMentioned: []string{"3.3-8", "3.3-10"},
		},
		{name: "nothing offered for the pin", offer: o, build: imagebuild.ManifestBuild{AirflowVersion: "3.2", LockedRuntime: "3.2-4"}, want: "3.2-4"},
		{name: "no offer", offer: nil, build: imagebuild.ManifestBuild{AirflowVersion: "3.3", LockedRuntime: "3.3-8"}, want: "3.3-8"},
		{name: "no lock build", offer: o, build: imagebuild.ManifestBuild{AirflowVersion: "3.3"}, want: "3.3-10"},
		{name: "runtime wins", offer: o, build: imagebuild.ManifestBuild{AirflowVersion: "3.3", Runtime: "3.3-7", LockedRuntime: "3.3-9"}, want: "3.3-7"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var warnings []string
			ref, err := OfferedBase(tt.offer, resolve, func(s string) { warnings = append(warnings, s) })(context.Background(), tt.build)
			require.NoError(t, err)
			assert.Equal(t, tt.want, ref)
			if !tt.wantWarn {
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
