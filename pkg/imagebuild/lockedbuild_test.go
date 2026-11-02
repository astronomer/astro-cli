package imagebuild

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLockedAirflow(t *testing.T) {
	assert.Equal(t, "3.3.2+astro.1", lockedAirflow([]string{"sqlalchemy==1.4.54", "apache-airflow==3.3.2+astro.1", "apache-airflow-core==3.3.2+astro.1"}))
	assert.Equal(t, "", lockedAirflow([]string{"apache-airflow==3.3.2"}), "not an +astro pin")
	assert.Equal(t, "", lockedAirflow(nil))
}

func TestSameAirflowRelease(t *testing.T) {
	assert.True(t, SameAirflowRelease("3.3.2+astro.1", "3.3.2+astro.4"))
	assert.True(t, SameAirflowRelease("3.3.2", "3.3.2+astro.4"))
	assert.True(t, SameAirflowRelease("", "3.3.2"), "unknown agrees")
	assert.False(t, SameAirflowRelease("3.3.2+astro.1", "3.3.1+astro.1"))
}

// With no [tool.astro] runtime, an image builds on the runtime version the
// lock was made for while the requirement covers its Airflow; a record the
// requirement has moved off, a named runtime, and a version that cannot be
// resolved all fall back to the base's own rule.
func TestLockedBase(t *testing.T) {
	var asked []string
	base := lockedBase(func(_ context.Context, m ManifestBuild) (string, error) {
		asked = append(asked, m.Runtime)
		if m.Runtime == "3.3-0" {
			return "", errors.New("no such runtime")
		}
		return "img:" + m.Runtime, nil
	})
	for _, tt := range []struct {
		name string
		m    ManifestBuild
		want string
	}{
		{"recorded", ManifestBuild{AirflowVersion: "3.3", LockedRuntime: "3.3-9", LockedAirflow: "3.3.1+astro.2"}, "img:3.3-9"},
		{"named runtime wins", ManifestBuild{AirflowVersion: "3.3", Runtime: "3.3-11", LockedRuntime: "3.3-9", LockedAirflow: "3.3.1+astro.2"}, "img:3.3-11"},
		{"requirement moved off", ManifestBuild{AirflowVersion: "3.4", LockedRuntime: "3.3-9", LockedAirflow: "3.3.1+astro.2"}, "img:"},
		{"no record", ManifestBuild{AirflowVersion: "3.3"}, "img:"},
		{"unresolvable", ManifestBuild{AirflowVersion: "3.3", LockedRuntime: "3.3-0", LockedAirflow: "3.3.0+astro.1"}, "img:"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := base(context.Background(), tt.m)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
