package deploybuild

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
)

// offer is a fake RuntimeOffer: newest maps a pin to its newest offered
// build, and airflow each offered build to its Airflow.
type offer struct {
	newest, airflow map[string]string
}

func (o offer) NewestFor(pin string) string {
	return o.newest[pin]
}

func (o offer) AirflowOf(build string) string {
	return o.airflow[build]
}

func resolveRecorder(got *[]string) imagebuild.BaseImageFunc {
	return func(_ context.Context, m imagebuild.ManifestBuild) (string, error) {
		*got = append(*got, m.Runtime)
		return "base:" + m.Runtime, nil
	}
}

// The manifest's runtime wins, then the offer's newest for the pin, and with
// neither the platform's own fallback runs with no build named.
func TestOfferedBase(t *testing.T) {
	var got []string
	resolve := resolveRecorder(&got)
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

// The runtime version uv.lock was made for is built on when the Deployment
// offers it. When it does not, the newest offered version stands in with one
// warning if it has the lock's Airflow release, and is refused before the
// build if it has another. A record the requirement no longer covers is not
// used.
func TestOfferedBaseLockedRuntime(t *testing.T) {
	o := offer{
		newest:  map[string]string{"3.3": "3.3-11"},
		airflow: map[string]string{"3.3-11": "3.3.2+astro.4", "3.3-9": "3.3.1+astro.2"},
	}
	locked := func(runtime, af string) imagebuild.ManifestBuild {
		return imagebuild.ManifestBuild{AirflowVersion: "3.3", LockedRuntime: runtime, LockedAirflow: af}
	}
	for _, tt := range []struct {
		name, want, warned, refused string
		m                           imagebuild.ManifestBuild
	}{
		{name: "offered", m: locked("3.3-9", "3.3.1+astro.2"), want: "3.3-9"},
		{
			name: "not offered, same Airflow", m: locked("3.3-10", "3.3.2+astro.3"), want: "3.3-11",
			warned: "uv.lock was made for Astro Runtime 3.3-10, which the Deployment does not offer, so the image builds on 3.3-11, which has the same Airflow",
		},
		{
			name: "not offered, other Airflow", m: locked("3.3-8", "3.3.0+astro.1"),
			refused: "uv.lock was made for Astro Runtime 3.3-8 (Airflow 3.3.0), and the Deployment does not offer it. The image would build on 3.3-11, which has Airflow 3.3.2",
		},
		{name: "record the requirement no longer covers", m: imagebuild.ManifestBuild{AirflowVersion: "3.3", LockedRuntime: "3.2-5", LockedAirflow: "3.2.1+astro.1"}, want: "3.3-11"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got, warnings []string
			ref, err := OfferedBase(o, resolveRecorder(&got), func(w string) { warnings = append(warnings, w) })(context.Background(), tt.m)
			if tt.refused != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.refused)
				assert.Empty(t, got, "refused before the base resolves")
				assert.Empty(t, warnings, "a refusal is not preceded by a warning")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "base:"+tt.want, ref)
			if tt.warned == "" {
				assert.Empty(t, warnings)
			} else {
				assert.Equal(t, []string{tt.warned}, warnings)
			}
		})
	}
}
