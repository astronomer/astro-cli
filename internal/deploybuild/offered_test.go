package deploybuild

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
)

type offer map[string]string

func (o offer) NewestFor(pin string) string { return o[pin] }

// The manifest's runtime wins, then the offer's newest for the pin, and with
// neither the platform's own fallback runs with no build named.
func TestOfferedBase(t *testing.T) {
	var got []string
	resolve := func(_ context.Context, m imagebuild.ManifestBuild) (string, error) {
		got = append(got, m.Runtime)
		return "base:" + m.Runtime, nil
	}
	base := OfferedBase(offer{"2.10": "12.6.0"}, resolve)

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

	ref, err := OfferedBase(nil, resolve)(context.Background(), imagebuild.ManifestBuild{AirflowVersion: "2.10"})
	require.NoError(t, err)
	assert.Equal(t, "base:", ref, "no offer is the fallback")
}
