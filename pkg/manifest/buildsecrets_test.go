package manifest

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParseBuildSecret(t *testing.T) {
	for _, tc := range []struct {
		spec string
		want BuildSecret
		err  string
	}{
		{spec: "id=netrc,env=NETRC_CONTENT", want: BuildSecret{ID: "netrc", Env: "NETRC_CONTENT"}},
		{spec: "id=pip,src=/etc/pip.conf", want: BuildSecret{ID: "pip", Src: "/etc/pip.conf"}},
		{spec: "ID=pip,Source=~/.pip.conf", want: BuildSecret{ID: "pip", Src: "~/.pip.conf"}},
		{spec: "id=netrc,type=env,src=NETRC_CONTENT", want: BuildSecret{ID: "netrc", Env: "NETRC_CONTENT"}},
		{spec: "id=netrc", want: BuildSecret{ID: "netrc"}},
		{spec: "hunter2", err: "not a key=value pair"},
		{spec: "id=netrc,value=hunter2", err: "a key other than id, src, env and type"},
		{spec: "id=netrc,type=ssh", err: "type other than file or env"},
		{spec: "id=netrc,env=machine github.com password hunter2", err: "not an environment variable name"},
		{spec: "id=netrc,type=env,src=hunter2 hunter2", err: "not an environment variable name"},
	} {
		t.Run(tc.spec, func(t *testing.T) {
			got, err := ParseBuildSecret(tc.spec)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("err = %v, want %q", err, tc.err)
				}
				if strings.Contains(err.Error(), "hunter2") {
					t.Errorf("the error repeats a value: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// build-secrets loads as written, and BuildSecretSpecs hands docker a src=
// under ~/ joined to the home directory, since docker does not expand it.
func TestBuildSecretSpecsExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	m, err := Load(write(t, "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\ndockerfile = 'Dockerfile'\n"+
		"build-secrets = ['id=netrc,env=NETRC_CONTENT', 'id=pip,src=~/.config/pip/pip.conf', 'id=ca,src=/etc/ca.pem']\n"))
	if err != nil {
		t.Fatal(err)
	}
	written := []string{"id=netrc,env=NETRC_CONTENT", "id=pip,src=~/.config/pip/pip.conf", "id=ca,src=/etc/ca.pem"}
	if !slices.Equal(m.Astro.BuildSecrets, written) {
		t.Errorf("BuildSecrets = %q, want %q", m.Astro.BuildSecrets, written)
	}
	want := []string{"id=netrc,env=NETRC_CONTENT", "id=pip,src=" + filepath.Join(home, ".config", "pip", "pip.conf"), "id=ca,src=/etc/ca.pem"}
	if got := m.Astro.BuildSecretSpecs(); !slices.Equal(got, want) {
		t.Errorf("BuildSecretSpecs = %q, want %q", got, want)
	}
}
