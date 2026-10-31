package imagebuild

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCreds answers for the hosts in it, and records every host asked.
type fakeCreds struct {
	have  map[string][2]string
	asked []string
}

func (f *fakeCreds) source(_ context.Context, host string) (login, password string, ok bool) {
	f.asked = append(f.asked, host)
	c, ok := f.have[host]
	return c[0], c[1], ok
}

// isolateHome points the home directory (HOME, and USERPROFILE, which Go
// reads on Windows) at a new empty directory, and returns it.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func gitRequest(secrets ...string) Request {
	return Request{
		Secrets: secrets,
		Install: &InstallLists{Requirements: []string{
			"corona @ git+https://github.com/example-org/corona.git@4bfeaf8",
			"humanize==4.9.0",
		}},
	}
}

func TestNetrcFromGitCredentialsForTheLocksHosts(t *testing.T) {
	isolateHome(t)
	creds := &fakeCreds{have: map[string][2]string{"github.com": {"x-access-token", "tok123"}}}
	b := New(nil, nil).WithCredentials(creds.source)

	got, err := b.withNetrc(context.Background(), gitRequest())
	require.NoError(t, err)
	assert.Equal(t, []string{"github.com"}, creds.asked)
	assert.Equal(t, []string{"id=netrc,env=" + netrcEnv}, got.Secrets)
	assert.Equal(t, []string{netrcEnv + "=machine github.com login x-access-token password tok123\n"}, got.Env)
	for _, s := range got.Secrets {
		assert.NotContains(t, s, "tok123", "the spec names a variable, never the secret")
	}
}

func TestNetrcDeclaredWithoutASourceIsResolved(t *testing.T) {
	isolateHome(t)
	creds := &fakeCreds{have: map[string][2]string{"github.com": {"u", "p"}}}
	got, err := New(nil, nil).WithCredentials(creds.source).withNetrc(context.Background(), gitRequest("id=netrc"))
	require.NoError(t, err)
	assert.Equal(t, []string{"id=netrc,env=" + netrcEnv}, got.Secrets)
}

func TestNetrcDeclaredRefusesAHostWithoutCredentials(t *testing.T) {
	isolateHome(t)
	_, err := New(nil, nil).WithCredentials((&fakeCreds{}).source).withNetrc(context.Background(), gitRequest("id=netrc"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github.com")
	assert.Contains(t, err.Error(), "--build-secret id=netrc,env=NETRC")
}

func TestNetrcOfferedIsLeftOutWithoutCredentials(t *testing.T) {
	isolateHome(t)
	got, err := New(nil, nil).WithCredentials((&fakeCreds{}).source).withNetrc(context.Background(), gitRequest())
	require.NoError(t, err, "a public host needs no credentials")
	assert.Empty(t, got.Secrets)
	assert.Empty(t, got.Env)
}

func TestNetrcPrefersTheHomeNetrc(t *testing.T) {
	home := isolateHome(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".netrc"), []byte("machine github.com login a password b\n"), 0o600))
	creds := &fakeCreds{}
	got, err := New(nil, nil).WithCredentials(creds.source).withNetrc(context.Background(), gitRequest("id=netrc"))
	require.NoError(t, err)
	assert.Empty(t, creds.asked)
	assert.Equal(t, []string{"id=netrc,src=" + filepath.Join(home, ".netrc")}, got.Secrets)
}

func TestNetrcWithASourceIsLeftAlone(t *testing.T) {
	isolateHome(t)
	creds := &fakeCreds{}
	req := gitRequest("id=netrc,env=NETRC")
	got, err := New(nil, nil).WithCredentials(creds.source).withNetrc(context.Background(), req)
	require.NoError(t, err)
	assert.Empty(t, creds.asked)
	assert.Equal(t, req.Secrets, got.Secrets)
}

func TestNetrcNothingToFetch(t *testing.T) {
	isolateHome(t)
	creds := &fakeCreds{}
	req := Request{Install: &InstallLists{Requirements: []string{"humanize==4.9.0"}}}
	got, err := New(nil, nil).WithCredentials(creds.source).withNetrc(context.Background(), req)
	require.NoError(t, err)
	assert.Empty(t, creds.asked)
	assert.Empty(t, got.Secrets)
}

func TestNetrcHostsReadsRoutedLinksAndADeclaredDockerfile(t *testing.T) {
	df := filepath.Join(t.TempDir(), "Dockerfile")
	require.NoError(t, os.WriteFile(df, []byte("FROM x\nRUN pip install \"pkg @ git+https://git.example.com/org/pkg.git\"\n"), 0o600))
	req := Request{Dockerfile: df, Install: &InstallLists{RoutedLinks: []string{"https://pkgs.example.com/simple/p/p-1.0-py3-none-any.whl#sha256=ab"}}}
	assert.Equal(t, []string{"git.example.com", "pkgs.example.com"}, req.netrcHosts())
}

func TestParseCredential(t *testing.T) {
	login, password, ok := parseCredential("protocol=https\nhost=github.com\nusername=u\npassword=p\n")
	assert.True(t, ok)
	assert.Equal(t, "u", login)
	assert.Equal(t, "p", password)
	_, _, ok = parseCredential("protocol=https\nhost=github.com\n")
	assert.False(t, ok)
	_, _, ok = parseCredential("username=u\npassword=" + strings.Repeat("a b", 2) + "\n")
	assert.False(t, ok, "a netrc token cannot hold whitespace")
}
