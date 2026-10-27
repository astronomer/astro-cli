package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A group joins default-groups when it is created, even with nothing in it,
// so it is installed once it fills; a project that took it out keeps it out.
func TestAGroupCreatedEmptyJoinsDefaultGroups(t *testing.T) {
	dir := projectWith(t, astroHead)
	_, err := SetRuntimeGroup(dir, nil, nil, &Seed{Build: "3.3-9", Python: "3.12"})
	require.NoError(t, err)
	assert.True(t, RuntimeGroupInstalled(dir), readManifest(t, dir))

	out := strings.Replace(readManifest(t, dir), "'astro-runtime',", "", 1)
	out = strings.Replace(out, ", 'astro-runtime'", "", 1)
	out = strings.Replace(out, "    \"astro-runtime\",\n", "", 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(out), 0o600))
	require.False(t, RuntimeGroupInstalled(dir), out)
	_, err = SetRuntimeGroup(dir, nil, &RuntimeGroup{Names: []string{"statsd"}}, nil)
	require.NoError(t, err)
	assert.False(t, RuntimeGroupInstalled(dir), "the group joined default-groups again after the project took it out:\n%s", readManifest(t, dir))
	assert.Contains(t, readManifest(t, dir), "'statsd'")
}

// RuntimeGroupAdds says whether a group lists a package the manifest's does
// not: what only a seed holds to the image's version.
func TestRuntimeGroupAdds(t *testing.T) {
	dir := projectWith(t, astroHead)
	_, err := SetRuntimeGroup(dir, nil, &RuntimeGroup{Names: []string{"statsd", "redis"}}, nil)
	require.NoError(t, err)
	assert.False(t, RuntimeGroupAdds(dir, &RuntimeGroup{Names: []string{"statsd"}}))
	assert.False(t, RuntimeGroupAdds(dir, &RuntimeGroup{Names: []string{"redis", "statsd"}}))
	assert.True(t, RuntimeGroupAdds(dir, &RuntimeGroup{Names: []string{"statsd", "pandas"}}))
	assert.False(t, RuntimeGroupAdds(dir, nil))
}
