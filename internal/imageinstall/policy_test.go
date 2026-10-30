package imageinstall

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Airflow and its core never install; nor does a Task SDK the project lists
// without a version, or one it does not list. Every line keeps its marker.
func TestApplyLeavesOutAirflow(t *testing.T) {
	for name, tc := range map[string]struct {
		deps string
		want []string
	}{
		"a bare Task SDK":           {`["apache-airflow==3.1.*", "Apache_Airflow-Core", "apache-airflow-task-sdk", "pandas"]`, []string{"pandas==2.2.3 ; python_version >= '3.10'"}},
		"a Task SDK with a version": {`["apache-airflow-task-sdk>=1.1", "pandas"]`, []string{"apache-airflow-task-sdk==1.1.5", "pandas==2.2.3 ; python_version >= '3.10'"}},
	} {
		t.Run(name, func(t *testing.T) {
			lists, warnings, err := apply(&policyInput{
				pyproject: []byte("[project]\nname = 'p'\ndependencies = " + tc.deps + "\n"),
				pylock: []byte(`
[[packages]]
name = "apache-airflow"
version = "3.1.5"
[[packages]]
name = "apache-airflow-core"
version = "3.1.5"
[[packages]]
name = "apache-airflow-task-sdk"
version = "1.1.5"
[[packages]]
name = "pandas"
version = "2.2.3"
marker = "python_version >= '3.10'"
[[packages]]
name = "numpy"
version = "2.1.0"
marker = "python_version >= '3.10'"
`),
				baseInstalled: []string{},
			})
			require.NoError(t, err)
			assert.Empty(t, warnings)
			assert.Equal(t, tc.want, lists.Requirements)
			assert.Equal(t, []string{"numpy==2.1.0 ; python_version >= '3.10'"}, lists.Constraints)
		})
	}
}

// A routed package installs on its own, from every file the lock recorded
// for it: the line pins its version with the lock's marker, and the page lists
// each wheel and the sdist with its hash, without the URL's credentials, for
// pip in the image to pick from. It installs this way whether the project
// names it or it is only a dependency, unless the image already has it. The
// routed install takes no dependencies, so requirements.txt names it too, at
// the same version: the main install finds it installed and brings the
// dependencies it needs.
func TestApplyInstallsARoutedPackageFromTheLocksFiles(t *testing.T) {
	const pyproject = "[project]\nname = 'p'\ndependencies = ['fast', 'needs-dep']\n" +
		"[tool.uv.sources]\nfast = [{ index = 'acme', marker = \"sys_platform == 'linux'\" }]\ndep = { index = 'acme' }\nshipped = { index = 'acme' }\n"
	lists, warnings, err := apply(&policyInput{
		pyproject: []byte(pyproject),
		pylock: []byte(`
[[packages]]
name = "fast"
version = "1.0"
marker = "sys_platform == 'linux'"
sdist = { url = "https://acme.example/fast-1.0.tar.gz", hashes = { sha256 = "s0" } }
wheels = [
  { url = "https://acme.example/fast-1.0-cp312-cp312-manylinux_2_28_x86_64.whl", hashes = { sha256 = "x1" } },
  { url = "https://user:pw@acme.example/fast-1.0-cp312-cp312-manylinux_2_34_aarch64.whl", hashes = { sha256 = "a0" } },
]
[[packages]]
name = "needs-dep"
version = "3.0"
[[packages]]
name = "dep"
version = "2.0"
wheels = [{ url = "https://acme.example/dep-2.0-py3-none-any.whl", hashes = { sha256 = "p0" } }]
[[packages]]
name = "shipped"
version = "4.0"
wheels = [{ url = "https://acme.example/shipped-4.0-py3-none-any.whl", hashes = { sha256 = "h0" } }]
`),
		baseInstalled: []string{"shipped==3.9"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"fast==1.0 ; sys_platform == 'linux'", "dep==2.0"}, lists.Routed)
	assert.Equal(t, []string{
		"https://acme.example/fast-1.0-cp312-cp312-manylinux_2_28_x86_64.whl#sha256=x1",
		"https://acme.example/fast-1.0-cp312-cp312-manylinux_2_34_aarch64.whl#sha256=a0",
		"https://acme.example/fast-1.0.tar.gz#sha256=s0",
		"https://acme.example/dep-2.0-py3-none-any.whl#sha256=p0",
	}, lists.RoutedLinks)
	assert.Equal(t, []string{"fast==1.0 ; sys_platform == 'linux'", "needs-dep==3.0", "dep==2.0"}, lists.Requirements,
		"a routed package is in requirements.txt too, so the main install brings its dependencies")
	assert.Empty(t, lists.Constraints)
	assert.Len(t, warnings, 1, "the stripped credentials are warned of")
}

// A routed package the lock recorded no file of installs by name.
func TestApplyInstallsARoutedPackageByNameWithoutAFile(t *testing.T) {
	lists, _, err := apply(&policyInput{
		pyproject: []byte("[project]\nname = 'p'\ndependencies = ['bare']\n[tool.uv.sources]\nbare = { index = 'acme' }\n"),
		pylock:    []byte("[[packages]]\nname = \"bare\"\nversion = \"1.0\"\n"),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"bare==1.0"}, lists.Requirements)
	assert.Empty(t, lists.Routed)
}

// statesVersion tells a requirement that limits its version from one that
// does not.
func TestStatesVersion(t *testing.T) {
	for req, want := range map[string]bool{
		"pandas":                        false,
		"pandas[sql]":                   false,
		"pandas ; python_version > '3'": false,
		"pandas @ https://x/p.whl":      false,
		"pandas>=2":                     true,
		"pandas[sql] ==2.2":             true,
		"pandas (>=2)":                  true,
		"apache-airflow-task-sdk~=1.1":  true,
	} {
		assert.Equal(t, want, statesVersion(req), req)
	}
}

// A URL's credentials are left out, and said to be, except an ssh URL's user
// with no password, which is the account ssh logs in as.
func TestURLCleaner(t *testing.T) {
	for raw, tc := range map[string]struct {
		want     string
		stripped bool
	}{
		"ssh://git@github.com/org/repo.git":          {"ssh://git@github.com/org/repo.git", false},
		"git+ssh://git@github.com/org/repo.git":      {"git+ssh://git@github.com/org/repo.git", false},
		"ssh://git:pw@github.com/org/repo.git":       {"ssh://github.com/org/repo.git", true},
		"https://token@github.com/org/repo.git":      {"https://github.com/org/repo.git", true},
		"https://user:pw@acme.example/p-1.0.whl":     {"https://acme.example/p-1.0.whl", true},
		"https://files.pythonhosted.org/p/p-1.0.whl": {"https://files.pythonhosted.org/p/p-1.0.whl", false},
	} {
		c := &urlCleaner{}
		assert.Equal(t, tc.want, c.clean(raw), raw)
		assert.Equal(t, tc.stripped, c.stripped, raw)
	}
}

// A workspace member, which the lock lists as a directory, is refused like a
// path source.
func TestApplyRefusesAWorkspaceMember(t *testing.T) {
	_, _, err := apply(&policyInput{
		pyproject: []byte("[project]\nname = 'p'\ndependencies = ['member']\n[tool.uv.sources]\nmember = { workspace = true }\n"),
		pylock:    []byte("[[packages]]\nname = \"member\"\nversion = \"0.1.0\"\ndirectory = { path = \"packages/member\", editable = true }\n"),
	})
	require.ErrorIs(t, err, ErrLocalSource)
	assert.Contains(t, err.Error(), "member (packages/member)")
}
