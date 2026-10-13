package pack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// A generated image and a declared Dockerfile's image keep the same
// per-machine files out of the registry. The rules live in two modules that do
// not import each other (imagebuild.ShippedImageIgnores, and the .dockerignore
// pkg/scaffold writes), so this holds the first to cover the second: each of
// scaffold's rules, made into a file, leaves a generated image's files as they
// were.
func TestShippedImagesLeaveOutWhatScaffoldIgnores(t *testing.T) {
	project := t.TempDir()
	write := func(rel, body string) {
		full := filepath.Join(project, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o600))
	}
	write("dags/a.py", "a = 1\n")
	before, err := imagebuild.ProjectFilesDigest(project)
	require.NoError(t, err)

	for _, rule := range scaffold.DockerignoreRules() {
		rel := strings.ReplaceAll(rule, "*", "machine")
		if strings.HasSuffix(rel, "/") {
			rel += "state"
		}
		write(rel, "per-machine\n")
	}
	after, err := imagebuild.ProjectFilesDigest(project)
	require.NoError(t, err)
	assert.Equal(t, before, after, "a generated image carried a file pkg/scaffold keeps out of a declared one: add its rule to imagebuild.ShippedImageIgnores")
}
