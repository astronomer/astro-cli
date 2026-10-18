package deploybuild

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A prebuilt image's runtime is held to the list more loosely than a build's:
// only a downgrade, which Houston refuses too, is refused. Airflow 3 where
// none is listed and a runtime newer than the newest listed are warned of and
// deployed, as Houston may take them, and an unlisted runtime is warned of as
// a build's is.
func TestPrebuiltListedRuntimeRules(t *testing.T) {
	listed := []string{"3.1-5", "3.0-9", "12.1.1", "11.9.0"}
	tests := []struct {
		name     string
		current  string
		tag      string
		listed   []string
		wantErr  string
		wantWarn string
	}{
		{name: "a listed runtime", current: "3.0-9", tag: "3.1-5", listed: listed},
		{name: "a downgrade", current: "3.1-5", tag: "3.0-9", listed: listed, wantErr: "cannot deploy Astro Runtime 3.0-9: it is a downgrade from the deployment's current 3.1-5; to deploy, raise to 3.1-5"},
		{name: "Airflow 3 to 2", current: "3.0-9", tag: "12.1.1", listed: listed, wantErr: "downgrade"},
		{name: "no list still refuses a downgrade", current: "3.0-9", tag: "12.1.1", wantErr: "downgrade"},
		{name: "newer than the newest listed warns", current: "3.1-5", tag: "3.2-1", listed: listed, wantWarn: "Astro Runtime 3.2-1 is newer than the newest the platform lists for this Deployment, 3.1-5 (it lists 3.1-5, 3.0-9, 12.1.1, 11.9.0). It deploys anyway"},
		{name: "Airflow 3 where none is listed warns", current: "12.1.1", tag: "3.1-5", listed: []string{"12.1.1", "11.9.0"}, wantWarn: "Astro Runtime 3.1-5 is Airflow 3, and the platform lists no Airflow 3 build for this Deployment (it lists 12.1.1, 11.9.0), so Airflow 3 may not be enabled for it. It deploys anyway"},
		{name: "an older build of a listed Airflow warns", current: "3.0-9", tag: "3.1-4", listed: listed, wantWarn: "Astro Runtime 3.1-4 is not one the platform lists for this Deployment"},
		{name: "no list warns of nothing", current: "3.0-9", tag: "3.9-1"},
	}
	p := Plan{raise: func(minimum string) string { return "raise to " + minimum }}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warning, err := p.CheckPrebuiltListed(tt.current, tt.tag, tt.listed)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.wantWarn == "" {
				assert.Empty(t, warning)
				return
			}
			assert.Contains(t, warning, tt.wantWarn)
		})
	}
}
