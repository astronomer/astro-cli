package pack

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

// initRepo makes dir a git repository whose .gitignore holds ignore.
func initRepo(t *testing.T, dir, ignore string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	cmd := exec.Command("git", "-C", dir, "init", "-q")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(ignore), 0o600))
}

// A packaged image is pushed to a registry as a deploy's is: a gitignored
// file it would carry that looks like a credential stops it before the build,
// for a generated build and a declared Dockerfile's alike, and other
// gitignored files are warned about.
func TestAstroBuildRefusesGitignoredCredentials(t *testing.T) {
	for name, req := range map[string]Request{
		"generated": testRequest(t),
		"declared":  declaringRequest(t, "Dockerfile", "FROM my-own-base\nCOPY . .\n"),
	} {
		t.Run(name, func(t *testing.T) {
			initRepo(t, req.ProjectDir, "certs/\ntarget/\n")
			require.NoError(t, os.MkdirAll(filepath.Join(req.ProjectDir, "certs"), 0o750))
			require.NoError(t, os.WriteFile(filepath.Join(req.ProjectDir, "certs", "prod.pem"), []byte("k"), 0o600))
			require.NoError(t, os.MkdirAll(filepath.Join(req.ProjectDir, "target"), 0o750))
			require.NoError(t, os.WriteFile(filepath.Join(req.ProjectDir, "target", "manifest.json"), []byte("{}"), 0o600))

			builder := &fakeBuilder{}
			var lines, warned []string
			cb := localrt.Callbacks{OnLine: func(l localrt.LogLine) { lines = append(lines, l.Text) }}
			req.Warn = func(w string) { warned = append(warned, w) }
			_, err := newAstro(builder, &fakeDocker{inspectOut: "3.1-2"}).Build(context.Background(), req, cb)
			require.ErrorContains(t, err, "certs/prod.pem looks like credentials")
			assert.Empty(t, builder.gotReq.Tag, "nothing was built")
			assert.Contains(t, strings.Join(warned, "\n"), "2 files that .gitignore ignores and .dockerignore does not, in certs/ and target/",
				"a refused build still hands over its warnings")

			require.NoError(t, os.WriteFile(filepath.Join(req.ProjectDir, ".dockerignore"), []byte("certs/\n**/.git\n"), 0o600))
			lines, warned = nil, nil
			res, err := newAstro(builder, &fakeDocker{inspectOut: "3.1-2"}).Build(context.Background(), req, cb)
			require.NoError(t, err)
			assert.Empty(t, lines, "warnings are not progress lines")
			assert.Equal(t, warned, res.Warnings, "-o json's result carries every warning shown")
			require.Len(t, warned, 1)
			assert.Equal(t, "the image will carry 1 file that .gitignore ignores and .dockerignore does not, in target/. If it does not belong in the image, add it to .dockerignore", warned[0])
		})
	}
}
