package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A shape the move would have to guess at is left as it is, with the reason.
func TestInstallBaseRewriteOfLeavesWhatItCannotRead(t *testing.T) {
	for _, tc := range []struct{ name, body, why string }{
		{"an earlier ONBUILD stage", "FROM quay.io/astronomer/astro-runtime:12.1.0 AS deps\nFROM quay.io/astronomer/astro-runtime:12.1.0\nRUN true\n", "an earlier stage builds on"},
		{"reads requirements.txt", "FROM quay.io/astronomer/astro-runtime:12.1.0\nRUN pip install -r requirements.txt\n", "it reads requirements.txt itself"},
		{"a continued FROM", "FROM quay.io/astronomer/astro-runtime:12.1.0 \\\n  AS final\nRUN true\n", "continues onto the next one"},
		{"a mirror named otherwise", "FROM registry.example/astronomer/airflow:3.1-12\nRUN true\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rw, why := installBaseRewriteOf([]byte(tc.body))
			assert.Nil(t, rw)
			if tc.why == "" {
				assert.Empty(t, why)
			} else {
				assert.Contains(t, why, tc.why)
			}
		})
	}
}

// A builder stage on another image is left above the moved final stage, and
// the final stage keeps its name.
func TestInstallBaseRewriteKeepsEarlierStages(t *testing.T) {
	rw, why := installBaseRewriteOf([]byte("FROM python:3.12-slim AS builder\nRUN make\n\n" +
		"FROM astrocrpublic.azurecr.io/runtime:3.1-12 AS final\nCOPY --from=builder /out /opt/out\n"))
	require.NotNil(t, rw, why)
	assert.Equal(t, "FROM python:3.12-slim AS builder\nRUN make\n\n"+installBaseHeader+
		"FROM localhost/astro-base AS final\n"+projectCopy+"\n\nCOPY --from=builder /out /opt/out\n", string(rw.body))
	assert.Equal(t, "3.1-12", rw.runtime)
}

// requirements.txt with a line pyproject.toml cannot hold keeps the
// Dockerfile on its ONBUILD tag, which still installs that line.
func TestAnUncarriedRequirementKeepsTheDockerfile(t *testing.T) {
	dir := t.TempDir()
	body := "FROM astrocrpublic.azurecr.io/runtime:3.1-12\nENV TZ=UTC\n"
	writeAll(t, dir, map[string]string{"Dockerfile": body, "requirements.txt": "-e ./libs/utils\npandas\n"})
	res, err := Run(dir, Options{})
	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
	assert.FileExists(t, filepath.Join(dir, "requirements.txt"))
	assert.Contains(t, strings.Join(res.Notes, "\n"), "Dockerfile: left as it is, because requirements.txt has lines")
}
