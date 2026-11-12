package localrt

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// pyproject builds a manifest from the [project] dependencies and whatever
// [tool] tables a case adds, parsed the way a start reads it.
func pyproject(t *testing.T, deps, tool string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.Parse([]byte("[project]\nname = \"p\"\nrequires-python = \">=3.11\"\ndependencies = [" + deps + "]\n\n[tool.astro]\n" + tool))
	require.NoError(t, err)
	return m
}

const baseDeps = `"apache-airflow==3.1.*", "pandas>=2.1"`

func TestClassifyChange(t *testing.T) {
	cases := []struct {
		name             string
		mode             Mode
		beforeDeps       string
		afterDeps        string
		beforeTool       string
		afterTool        string
		wantAction       ChangeAction
		wantInstall      []string
		wantRemoved      []string
		wantReasonsCount int
	}{
		{
			name:       "nothing changed",
			beforeDeps: baseDeps, afterDeps: baseDeps,
			wantAction: ChangeNone,
		},
		{
			name:       "reordered dependencies are not a change",
			beforeDeps: baseDeps, afterDeps: `"pandas>=2.1", "apache-airflow==3.1.*"`,
			wantAction: ChangeNone,
		},
		{
			name:       "a link is not something Airflow reads",
			beforeDeps: baseDeps, afterDeps: baseDeps,
			afterTool:  "[tool.astro.deployments.prod]\ndeployment = \"dep-prod\"\nworkspace = \"ws\"\n",
			wantAction: ChangeNone,
		},
		{
			name:       "an added package hot installs",
			beforeDeps: baseDeps, afterDeps: baseDeps + `, "requests"`,
			wantAction:  ChangeHotInstall,
			wantInstall: []string{"requests"},
		},
		{
			name:       "a re-pinned package is a restart, since uv would replace it under the scheduler",
			beforeDeps: baseDeps, afterDeps: `"apache-airflow==3.1.*", "pandas>=2.2"`,
			wantAction: ChangeRestart, wantReasonsCount: 1,
		},
		{
			name:       "a downgrade is a restart",
			beforeDeps: baseDeps, afterDeps: `"apache-airflow==3.1.*", "pandas<2"`,
			wantAction: ChangeRestart, wantReasonsCount: 1,
		},
		{
			name:       "respacing a requirement is not a change",
			beforeDeps: baseDeps, afterDeps: `"apache-airflow == 3.1.*", "pandas >= 2.1"`,
			wantAction: ChangeNone,
		},
		{
			name:       "a name's case and separators are not a change",
			beforeDeps: baseDeps + `, "typing_extensions"`, afterDeps: `"Apache-Airflow==3.1.*", "Pandas>=2.1", "typing-extensions"`,
			wantAction: ChangeNone,
		},
		{
			name:       "respacing a marker outside its quotes is not a change",
			beforeDeps: baseDeps + `, "pywin32; sys_platform == 'win32'"`, afterDeps: baseDeps + `, "pywin32;sys_platform=='win32'"`,
			wantAction: ChangeNone,
		},
		{
			name:       "an added package from a git source is a restart, since uv pip cannot read the source",
			beforeDeps: baseDeps, afterDeps: baseDeps + `, "my_lib"`,
			beforeTool: "\n[tool.uv.sources]\nmy-lib = { git = \"https://example.com/my-lib.git\" }\n",
			afterTool:  "\n[tool.uv.sources]\nmy-lib = { git = \"https://example.com/my-lib.git\" }\n",
			wantAction: ChangeRestart, wantReasonsCount: 1,
		},
		{
			name:       "an added package from a path source is a restart",
			beforeDeps: baseDeps, afterDeps: baseDeps + `, "my-lib"`,
			beforeTool: "\n[tool.uv.sources]\nmy-lib = { path = \"../my-lib\" }\n",
			afterTool:  "\n[tool.uv.sources]\nmy-lib = { path = \"../my-lib\" }\n",
			wantAction: ChangeRestart, wantReasonsCount: 1,
		},
		{
			name:       "an override changes how everything resolves",
			beforeDeps: baseDeps, afterDeps: baseDeps,
			afterTool:  "\n[tool.uv]\noverride-dependencies = [\"pydantic<2\"]\n",
			wantAction: ChangeRestart, wantReasonsCount: 1,
		},
		{
			name:       "a new default index changes how everything resolves",
			beforeDeps: baseDeps, afterDeps: baseDeps,
			afterTool:  "\n[tool.uv]\nindex-url = \"https://mirror.example.com/simple\"\n",
			wantAction: ChangeRestart, wantReasonsCount: 1,
		},
		{
			name:       "reordering constraints is not a change",
			beforeDeps: baseDeps, afterDeps: baseDeps,
			beforeTool: "\n[tool.uv]\nconstraint-dependencies = [\"urllib3<2\", \"idna>=3\"]\n",
			afterTool:  "\n[tool.uv]\nconstraint-dependencies = [\"idna >= 3\", \"urllib3<2\"]\n",
			wantAction: ChangeNone,
		},
		{
			name:       "a re-linked workspace changes the env values read from it",
			beforeDeps: baseDeps, afterDeps: baseDeps,
			beforeTool: "workspace = \"ws-one\"\n[tool.astro.env.FOO]\ndefault = \"bar\"\n",
			afterTool:  "workspace = \"ws-two\"\n[tool.astro.env.FOO]\ndefault = \"bar\"\n",
			wantAction: ChangeRestart, wantReasonsCount: 1,
		},
		{
			name:       "a re-linked workspace with no env is nothing Airflow reads",
			beforeDeps: baseDeps, afterDeps: baseDeps,
			beforeTool: "workspace = \"ws-one\"\n",
			afterTool:  "workspace = \"ws-two\"\n",
			wantAction: ChangeNone,
		},
		{
			name:       "a provider is an ordinary package",
			beforeDeps: baseDeps, afterDeps: baseDeps + `, "apache-airflow-providers-snowflake"`,
			wantAction:  ChangeHotInstall,
			wantInstall: []string{"apache-airflow-providers-snowflake"},
		},
		{
			name:       "a removal alone needs nothing, and is reported",
			beforeDeps: baseDeps + `, "requests"`, afterDeps: baseDeps,
			wantAction:  ChangeNone,
			wantRemoved: []string{"requests"},
		},
		{
			name:       "an addition beside a removal hot installs and reports both",
			beforeDeps: baseDeps + `, "requests"`, afterDeps: baseDeps + `, "httpx"`,
			wantAction:  ChangeHotInstall,
			wantInstall: []string{"httpx"},
			wantRemoved: []string{"requests"},
		},
		{
			name:       "a new Airflow is a restart",
			beforeDeps: baseDeps, afterDeps: `"apache-airflow==3.2.*", "pandas>=2.1"`,
			wantAction: ChangeRestart, wantReasonsCount: 1,
		},
		{
			name:       "the task SDK is part of Airflow",
			beforeDeps: baseDeps, afterDeps: baseDeps + `, "apache-airflow-task-sdk>=1.1"`,
			wantAction: ChangeRestart, wantReasonsCount: 1,
		},
		{
			name:       "a new constraint changes how everything resolves",
			beforeDeps: baseDeps, afterDeps: baseDeps + `, "requests"`,
			afterTool:  "\n[tool.uv]\nconstraint-dependencies = [\"urllib3<2\"]\n",
			wantAction: ChangeRestart, wantReasonsCount: 1,
		},
		{
			name:       "env is read at start",
			beforeDeps: baseDeps, afterDeps: baseDeps,
			afterTool:  "[tool.astro.env.FOO]\ndefault = \"bar\"\n",
			wantAction: ChangeRestart, wantReasonsCount: 1,
		},
		{
			name:       "pools are applied at start",
			beforeDeps: baseDeps, afterDeps: baseDeps,
			afterTool:  "[tool.astro.pools.etl]\nslots = 4\n",
			wantAction: ChangeRestart, wantReasonsCount: 1,
		},
		{
			name:       "standalone ignores apt packages",
			beforeDeps: baseDeps, afterDeps: baseDeps,
			afterTool:  "packages = [\"libpq-dev\"]\n",
			wantAction: ChangeNone,
		},
		{
			name:       "docker installs apt packages into the image",
			mode:       ModeDocker,
			beforeDeps: baseDeps, afterDeps: baseDeps,
			afterTool:  "packages = [\"libpq-dev\"]\n",
			wantAction: ChangeRestart, wantReasonsCount: 1,
		},
		{
			name:       "docker rebuilds for an added package",
			mode:       ModeDocker,
			beforeDeps: baseDeps, afterDeps: baseDeps + `, "requests"`,
			wantAction: ChangeRestart, wantReasonsCount: 1,
		},
		{
			name:       "docker rebuilds for a removal too",
			mode:       ModeDocker,
			beforeDeps: baseDeps + `, "requests"`, afterDeps: baseDeps,
			wantAction:  ChangeRestart,
			wantRemoved: []string{"requests"}, wantReasonsCount: 1,
		},
		{
			name:       "every reason is reported",
			mode:       ModeDocker,
			beforeDeps: baseDeps, afterDeps: `"apache-airflow==3.2.*", "pandas>=2.1", "requests"`,
			afterTool:  "packages = [\"libpq-dev\"]\n",
			wantAction: ChangeRestart, wantReasonsCount: 3,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := pyproject(t, tc.beforeDeps, tc.beforeTool)
			after := pyproject(t, tc.afterDeps, tc.afterTool)
			got := ClassifyChange(before, after, tc.mode)
			require.Equal(t, tc.wantAction, got.Action, "reasons: %v", got.Reasons)
			require.Equal(t, tc.wantInstall, got.Install)
			require.Equal(t, tc.wantRemoved, got.Removed)
			require.Len(t, got.Reasons, tc.wantReasonsCount, "reasons: %v", got.Reasons)
		})
	}
}

// The restart wins over the install: a hot install alongside a restart would
// install into the environment the restart is about to rebuild.
func TestClassifyChangeRestartCarriesNoInstall(t *testing.T) {
	before := pyproject(t, baseDeps, "")
	after := pyproject(t, `"apache-airflow==3.2.*", "pandas>=2.1", "requests"`, "")
	got := ClassifyChange(before, after, ModeStandalone)
	require.Equal(t, ChangeRestart, got.Action)
	require.Empty(t, got.Install)
}

// A watcher that has no manifest to compare must not panic, and must not
// guess that nothing changed.
func TestClassifyChangeWithoutAManifestRestarts(t *testing.T) {
	m := pyproject(t, baseDeps, "")
	for _, pair := range [][2]*manifest.Manifest{{nil, m}, {m, nil}, {nil, nil}} {
		got := ClassifyChange(pair[0], pair[1], ModeStandalone)
		require.Equal(t, ChangeRestart, got.Action)
		require.Equal(t, []Reason{{Code: ReasonUncompared}}, got.Reasons)
	}
}

// A caller that words reasons itself, as Desktop does, branches on the code
// and the distribution, so those are part of the answer and not only the text.
func TestClassifyChangeReasonCodes(t *testing.T) {
	src := "\n[tool.uv.sources]\nmy-lib = { git = \"https://example.com/my-lib.git\" }\n"
	before := pyproject(t, baseDeps, src)
	after := pyproject(t, `"apache-airflow==3.2.*", "Pandas>=2.2", "my_lib"`, src+"\n[tool.uv]\noverride-dependencies = [\"pydantic<2\"]\n")
	got := ClassifyChange(before, after, ModeStandalone)
	require.Equal(t, []Reason{
		{Code: ReasonAirflow},
		{Code: ReasonRepinned, Dist: "pandas"},
		{Code: ReasonSourced, Dist: "my-lib"},
		{Code: ReasonUV},
	}, got.Reasons)

	docker := ClassifyChange(pyproject(t, baseDeps, ""), pyproject(t, baseDeps+`, "requests"`, "dockerfile = \"Dockerfile\"\n"), ModeDocker)
	require.Equal(t, []Reason{{Code: ReasonImageDependencies}, {Code: ReasonDockerfile}}, docker.Reasons)
}

// Every code has its own words: one that fell through to the bare code would
// print "uv" to a user.
func TestReasonStringCoversEveryCode(t *testing.T) {
	codes := []ReasonCode{
		ReasonUncompared, ReasonRequiresPython, ReasonAirflow, ReasonImageDependencies,
		ReasonRepinned, ReasonSourced, ReasonUV, ReasonEnv, ReasonWorkspace, ReasonPools,
		ReasonPackages, ReasonDockerfile, ReasonRuntime, ReasonBuildSecrets,
		ReasonDotenv, ReasonDockerfileEdited, ReasonHotInstallUnavailable,
	}
	seen := map[string]bool{}
	for _, code := range codes {
		s := Reason{Code: code, Dist: "pandas"}.String()
		require.NotEqual(t, string(code), s)
		require.False(t, seen[s], "two codes read %q", s)
		seen[s] = true
	}
	require.Equal(t, "the requirement on pandas changed, and it is already installed in the running Airflow",
		Reason{Code: ReasonRepinned, Dist: "pandas"}.String())
}

// The JSON a desktop frontend receives: codes, not sentences.
func TestChangeJSON(t *testing.T) {
	b, err := json.Marshal(Change{Action: ChangeRestart, Reasons: []Reason{{Code: ReasonRepinned, Dist: "pandas"}, {Code: ReasonUV}}})
	require.NoError(t, err)
	require.JSONEq(t, `{"action":"restart","reasons":[{"code":"repinned","dist":"pandas"},{"code":"uv"}]}`, string(b))
}
