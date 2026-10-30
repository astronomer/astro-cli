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

// A routed package's file is the one the image's platform installs: one line
// when every platform takes the same file, else one for each, limited to it.
func TestApplyPicksARoutedPackagesFileForEachPlatform(t *testing.T) {
	const pyproject = "[project]\nname = 'p'\ndependencies = ['fast', 'pure']\n" +
		"[tool.uv.sources]\nfast = [{ index = 'acme', marker = \"sys_platform == 'linux'\" }]\npure = { index = 'acme' }\n"
	lists, _, err := apply(&policyInput{
		pyproject: []byte(pyproject),
		pylock: []byte(`
[[packages]]
name = "fast"
version = "1.0"
marker = "sys_platform == 'linux'"
sdist = { url = "https://acme.example/fast-1.0.tar.gz", hashes = { sha256 = "s0" } }
wheels = [
  { url = "https://acme.example/fast-1.0-cp312-cp312-win_amd64.whl", hashes = { sha256 = "w0" } },
  { url = "https://acme.example/fast-1.0-cp312-cp312-manylinux_2_17_x86_64.manylinux2014_x86_64.whl", hashes = { sha256 = "x0" } },
  { url = "https://acme.example/fast-1.0-cp312-cp312-manylinux_2_28_x86_64.whl", hashes = { sha256 = "x1" } },
  { url = "https://acme.example/fast-1.0-cp312-cp312-manylinux_2_34_aarch64.whl", hashes = { sha256 = "a0" } },
  { url = "https://acme.example/fast-1.0-cp313-cp313-manylinux_2_17_aarch64.whl", hashes = { sha256 = "a1" } },
]
[[packages]]
name = "pure"
version = "2.0"
wheels = [
  { url = "https://acme.example/pure-2.0-py2.py3-none-any.whl", hashes = { sha256 = "p0" } },
  { url = "https://acme.example/pure-2.0-cp312-abi3-manylinux_2_17_x86_64.whl", hashes = { sha256 = "p1" } },
]
`),
		python: "3.12",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"fast @ https://acme.example/fast-1.0-cp312-cp312-manylinux_2_28_x86_64.whl#sha256=x1 ; (sys_platform == 'linux') and platform_machine == 'x86_64'",
		// Neither aarch64 wheel installs: one needs glibc 2.34, the other
		// Python 3.13.
		"fast @ https://acme.example/fast-1.0.tar.gz#sha256=s0 ; (sys_platform == 'linux') and platform_machine == 'aarch64'",
		"pure @ https://acme.example/pure-2.0-cp312-abi3-manylinux_2_17_x86_64.whl#sha256=p1 ; platform_machine == 'x86_64'",
		"pure @ https://acme.example/pure-2.0-py2.py3-none-any.whl#sha256=p0 ; platform_machine == 'aarch64'",
	}, lists.Requirements)
}

// A routed package with no file the image can install installs by name.
func TestApplyInstallsARoutedPackageByNameWithoutAFile(t *testing.T) {
	lists, _, err := apply(&policyInput{
		pyproject: []byte("[project]\nname = 'p'\ndependencies = ['win']\n[tool.uv.sources]\nwin = { index = 'acme' }\n"),
		pylock: []byte(`
[[packages]]
name = "win"
version = "1.0"
wheels = [{ url = "https://acme.example/win-1.0-cp312-cp312-win_amd64.whl" }]
`),
		python: "3.12",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"win==1.0"}, lists.Requirements)
}

// The tags an image's CPython takes, best first, as packaging ranks them.
func TestImageTags(t *testing.T) {
	tags := imageTags("3.12", "x86_64")
	order := []string{
		"cp312-cp312-manylinux_2_28_x86_64",
		"cp312-cp312-manylinux2014_x86_64",
		"cp312-cp312-linux_x86_64",
		"cp312-abi3-manylinux_2_28_x86_64",
		"cp312-none-manylinux_2_28_x86_64",
		"cp311-abi3-manylinux_2_28_x86_64",
		"py312-none-manylinux_2_28_x86_64",
		"py3-none-linux_x86_64",
		"cp312-none-any",
		"py312-none-any",
		"py3-none-any",
		"py30-none-any",
	}
	for i := 1; i < len(order); i++ {
		a, okA := tags[order[i-1]]
		b, okB := tags[order[i]]
		require.True(t, okA && okB, "%s, %s", order[i-1], order[i])
		assert.Less(t, a, b, "%s before %s", order[i-1], order[i])
	}
	for _, no := range []string{"cp313-cp313-manylinux_2_17_x86_64", "cp312-cp312-manylinux_2_34_x86_64", "cp312-cp312-manylinux_2_17_aarch64", "cp312-cp312-win_amd64"} {
		assert.NotContains(t, tags, no)
	}
	assert.Contains(t, imageTags("", "aarch64"), "py3-none-any")
	assert.NotContains(t, imageTags("", "aarch64"), "cp312-none-any")
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
