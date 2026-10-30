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
// names it or it is only a dependency, unless the image already has it.
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
	assert.Equal(t, []string{"needs-dep==3.0"}, lists.Requirements, "a routed package is not in requirements.txt")
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
