package localshared

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/proxy"
)

// PlanHostname reports the name a project asks for. What it gets is settled
// by proxy.Store.AddRoute, under the routes lock — resolve_test.go there has
// the collision rules, and the engines' own tests have the wiring that
// carries the answer back to the state record.

func TestPlanHostnamePrefersThePlan(t *testing.T) {
	got, err := PlanHostname(rt.Plan{Hostname: "chosen.localhost"}, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, "chosen.localhost", got)
}

func TestPlanHostnameDerivesWhenThePlanHasNoName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "analytics")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	got, err := PlanHostname(rt.Plan{}, dir)
	require.NoError(t, err)
	assert.Equal(t, "analytics"+proxy.LocalhostSuffix, got)
}

// The discriminator is what lets AddRoute tell two projects of the same name
// apart, so it has to differ between directories and be the same for one
// directory on every call. One that moved would hand a project a different
// name on every restart, and strand the route registered under the old one.
func TestHostnameDiscriminatorIsStableAndPerProject(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()

	first := HostnameDiscriminator(a)
	assert.Len(t, first, proxy.HostnameIDLen)
	assert.Equal(t, first, HostnameDiscriminator(a), "the same project, the same discriminator")
	assert.NotEqual(t, first, HostnameDiscriminator(b), "two projects, two discriminators")
}

// One directory reached two ways is one project, so it must not get two
// discriminators — AddRoute would then read a restart as a stranger.
func TestHostnameDiscriminatorFollowsTheCanonicalPath(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	assert.Equal(t, HostnameDiscriminator(target), HostnameDiscriminator(link))
}

// A path with no id yields no discriminator, which AddRoute reads as "no way
// to tell these two apart" and refuses the duplicate — the behavior that was
// there before any of this. A display name never fails a start.
func TestHostnameDiscriminatorIsEmptyForAnUnresolvablePath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-dir", "project")
	assert.Empty(t, HostnameDiscriminator(missing))
}
