package airflowrt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecretMounts(t *testing.T) {
	tests := []struct {
		name, body string
		want       []SecretMount
	}{
		{
			name: "one mount",
			body: "FROM astrocrpublic.azurecr.io/runtime:3.3-8\nRUN --mount=type=secret,id=netrc,dst=/root/.netrc pip install -r r.txt\n",
			want: []SecretMount{{ID: "netrc", Line: 2}},
		},
		{
			name: "continued over lines with a comment inside",
			body: "FROM x\n\nRUN \\\n  # private index\n  --mount=type=secret,id=pip,target=/etc/pip.conf \\\n  pip install foo\n",
			want: []SecretMount{{ID: "pip", Line: 3}},
		},
		{
			name: "several mounts on one RUN, a cache mount among them",
			body: "FROM x\nRUN --mount=type=cache,target=/root/.cache --mount=type=secret,id=a --mount=type=secret,id=b make\n",
			want: []SecretMount{{ID: "a", Line: 2}, {ID: "b", Line: 2}},
		},
		{
			name: "no id takes the target's base name",
			body: "FROM x\nRUN --mount=type=secret,target=/run/secrets/token cat /run/secrets/token\n",
			want: []SecretMount{{ID: "token", Line: 2}},
		},
		{
			name: "the source is the id",
			body: "FROM x\nRUN --mount=type=secret,src=netrc,target=/root/.netrc pip install foo\n",
			want: []SecretMount{{ID: "netrc", Line: 2}},
		},
		{
			name: "an id from a build argument is left out",
			body: "FROM x\nARG SID=netrc\nRUN --mount=type=secret,id=${SID} a\n",
		},
		{
			name: "lower-case run, one entry per id",
			body: "FROM x\nrun --mount=type=secret,id=netrc a\nRUN --mount=type=secret,id=netrc b\n",
			want: []SecretMount{{ID: "netrc", Line: 2}},
		},
		{
			name: "a mount after the command is not a flag",
			body: "FROM x\nRUN echo --mount=type=secret,id=nope\n",
		},
		{
			name: "no mounts",
			body: "FROM x\nRUN pip install foo\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Dockerfile")
			require.NoError(t, os.WriteFile(path, []byte(tt.body), 0o600))
			got, err := SecretMounts(path)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
