package local

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pickRun is what a picker case reads back: the output streams, and what the
// fake Astro pickers were asked.
type pickRun struct {
	out, errOut          *bytes.Buffer
	deploymentWorkspaces []string
	linked               map[string]string
	pickErr              error
	workspacePicks       int
}

// pickDeps is linkDeps for a run that may prompt: a terminal, stdin holding
// the scripted answers, and fake Astro pickers that record what they were
// asked. A workspace picker the case does not expect fails the test.
func pickDeps(t *testing.T, dir, stdin string, dep PickedDeployment, ws string) (Deps, *pickRun) {
	t.Helper()
	d, out, errOut := linkDeps(t, dir)
	run := &pickRun{out: out, errOut: errOut}
	d.Interactive = func() bool { return true }
	d.Stdin = strings.NewReader(stdin)
	d.CurrentWorkspace = func() string { return "" }
	d.PickDeployment = func(workspaceID string, linked map[string]string) (PickedDeployment, error) {
		run.deploymentWorkspaces = append(run.deploymentWorkspaces, workspaceID)
		run.linked = linked
		if run.pickErr != nil {
			return PickedDeployment{}, run.pickErr
		}
		return dep, nil
	}
	d.PickWorkspace = func() (string, error) {
		run.workspacePicks++
		if ws == "" {
			t.Error("asked for a workspace the case did not expect")
		}
		return ws, nil
	}
	return d, run
}

// With no NAME and no --deployment, add asks which Deployment in the project's
// workspace to link, and names the link after it.
func TestLinkAddPicksADeploymentAndNamesTheLink(t *testing.T) {
	dir, path := linkTestProject(t, linkManifest)
	d, run := pickDeps(t, dir, "", PickedDeployment{ID: "clx9", Name: "Orders Prod (EU)", WorkspaceID: "ws_A"}, "")
	require.NoError(t, execute(t, d, "link", "add"))
	assert.Equal(t, []string{"ws_A"}, run.deploymentWorkspaces)
	assert.Contains(t, run.out.String(), "added link orders-prod-eu to "+path)
	link := loadManifest(t, path).Astro.Deployments["orders-prod-eu"]
	assert.Equal(t, "clx9", link.Deployment)
	assert.Equal(t, "ws_A", link.Workspace)
	assert.Equal(t, 1, strings.Count(readManifest(t, dir), "'ws_A'"), "the link repeated the project's workspace")

	// A NAME given is kept.
	d, _ = pickDeps(t, dir, "", PickedDeployment{ID: "clx8", Name: "Anything", WorkspaceID: "ws_A"}, "")
	require.NoError(t, execute(t, d, "link", "add", "mine"))
	assert.Equal(t, "clx8", loadManifest(t, path).Astro.Deployments["mine"].Deployment)

	out, _, err := runLink(t, dir, "add", "mine", "--deployment", "clx7", "--replace")
	require.NoError(t, err)
	assert.Equal(t, "replaced link mine in "+path+"\n", out)
}

// A project with no workspace lists the current login's, and with none of
// those either it asks for a workspace first; the link then names it.
func TestLinkAddPicksTheWorkspaceToListFrom(t *testing.T) {
	const bare = "[project]\nname = 'x'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\n"

	dir, path := linkTestProject(t, bare)
	d, run := pickDeps(t, dir, "", PickedDeployment{ID: "clx1", Name: "dev", WorkspaceID: "ws_C"}, "")
	d.CurrentWorkspace = func() string { return "ws_C" }
	require.NoError(t, execute(t, d, "link", "add"))
	assert.Equal(t, []string{"ws_C"}, run.deploymentWorkspaces)
	assert.Equal(t, "ws_C", loadManifest(t, path).Astro.Deployments["dev"].Workspace)

	dir, path = linkTestProject(t, bare)
	d, run = pickDeps(t, dir, "", PickedDeployment{ID: "clx2", Name: "dev"}, "ws_P")
	require.NoError(t, execute(t, d, "link", "add"))
	assert.Equal(t, 1, run.workspacePicks)
	assert.Equal(t, []string{"ws_P"}, run.deploymentWorkspaces)
	assert.Equal(t, "ws_P", loadManifest(t, path).Astro.Deployments["dev"].Workspace)

	// --workspace names the list outright.
	dir, _ = linkTestProject(t, linkManifest)
	d, run = pickDeps(t, dir, "", PickedDeployment{ID: "clx3", Name: "other"}, "")
	require.NoError(t, execute(t, d, "link", "add", "--workspace", "ws_Z"))
	assert.Equal(t, []string{"ws_Z"}, run.deploymentWorkspaces)
}

// A run that cannot be asked never prompts: no terminal, or json output. It
// names the missing argument instead, and writes nothing.
func TestLinkPickersNeverPromptARunThatCannotBeAsked(t *testing.T) {
	for name, tc := range map[string]struct {
		interactive bool
		args        []string
		want        string
	}{
		"add, no terminal":       {false, []string{"add"}, "add needs a NAME"},
		"add, json":              {true, []string{"add", "--output", "json"}, "add needs a NAME"},
		"add named, json":        {true, []string{"add", "prod", "--output", "json"}, "needs a deployment"},
		"remove, no terminal":    {false, []string{"remove"}, "astro link remove NAME"},
		"remove, json":           {true, []string{"remove", "--output", "json"}, "astro link remove NAME"},
		"default, no terminal":   {false, []string{"default"}, "--unset to clear it"},
		"default, json":          {true, []string{"default", "--output", "json"}, "--unset to clear it"},
		"add url with no NAME":   {true, []string{"add", "--url", "https://a.example.com", "--auth", "none"}, "add needs a NAME"},
		"add, --deployment only": {true, []string{"add", "--deployment", "clx1"}, "add needs a NAME"},
	} {
		t.Run(name, func(t *testing.T) {
			dir, _ := linkTestProject(t, linkManifest)
			d, run := pickDeps(t, dir, "1\n", PickedDeployment{ID: "clx9", Name: "x"}, "")
			d.Interactive = func() bool { return tc.interactive }
			err := execute(t, d, append([]string{"link"}, tc.args...)...)
			require.Error(t, err)
			if slices.Contains(tc.args, "json") {
				var e jsonError
				require.NoError(t, json.Unmarshal(run.out.Bytes(), &e), "decode %q", run.out.String())
				assert.Contains(t, e.Error, tc.want)
			} else {
				assert.Contains(t, err.Error(), tc.want)
			}
			assert.Empty(t, run.deploymentWorkspaces, "a picker ran")
			assert.Equal(t, linkManifest, readManifest(t, dir))
		})
	}
}

// A Deployment whose name leaves no link name asks for one.
// A NAME already linked is refused before any picker runs or the platform is
// called, and --replace goes on to pick.
func TestLinkAddRefusesAnExistingNameBeforePicking(t *testing.T) {
	dir, _ := linkTestProject(t, linkManifest)
	d, run := pickDeps(t, dir, "", PickedDeployment{ID: "clx9", Name: "x"}, "")
	err := execute(t, d, "link", "add", "dev")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a link named dev already exists")
	assert.Empty(t, run.deploymentWorkspaces, "a picker ran for a name that was refused")
	assert.Zero(t, run.workspacePicks)

	d, run = pickDeps(t, dir, "", PickedDeployment{ID: "clx9", Name: "x"}, "")
	require.NoError(t, execute(t, d, "link", "add", "dev", "--replace"))
	assert.Equal(t, []string{"ws_A"}, run.deploymentWorkspaces)
}

// Without the Astro pickers, as under a Software context, add with no
// --deployment asks for what it needs rather than picking, in a terminal too.
func TestLinkAddWithoutPickersNamesWhatIsMissing(t *testing.T) {
	dir, _ := linkTestProject(t, linkManifest)
	d, _ := pickDeps(t, dir, "", PickedDeployment{}, "")
	d.PickDeployment, d.PickWorkspace, d.CurrentWorkspace = nil, nil, nil
	err := execute(t, d, "link", "add")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "add needs a NAME")
	err = execute(t, d, "link", "add", "prod")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "needs a deployment")
	assert.Equal(t, linkManifest, readManifest(t, dir))
}

func TestLinkAddRefusesADeploymentNameThatMakesNoLinkName(t *testing.T) {
	for _, depName := range []string{"!!!", "Local"} {
		dir, _ := linkTestProject(t, linkManifest)
		d, _ := pickDeps(t, dir, "", PickedDeployment{ID: "clx9", Name: depName}, "")
		err := execute(t, d, "link", "add")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "astro link add NAME --deployment clx9")
		assert.Equal(t, linkManifest, readManifest(t, dir))
	}
}

func TestLinkNameFor(t *testing.T) {
	for in, want := range map[string]string{
		"Orders Prod (EU)": "orders-prod-eu",
		"  dev  ":          "dev",
		"a__b--c":          "a-b-c",
		"ÉTL 2":            "tl-2",
	} {
		assert.Equal(t, want, linkNameFor(in), in)
	}
}

// assertPickerTable checks the picker is the table picker: the title, the
// header, each row by number, and the "> " prompt after a blank line.
func assertPickerTable(t *testing.T, out, title string, rows ...string) {
	t.Helper()
	assert.True(t, strings.HasPrefix(out, title+"\n"), "no %q title line:\n%s", title, out)
	assert.Regexp(t, `#\s+NAME\s+KIND\s+DEPLOYMENT, ENVIRONMENT OR URL`, out)
	for _, row := range rows {
		assert.Regexp(t, row, out)
	}
	assert.Contains(t, out, "\n\n> ")
}

// With no NAME, remove asks which of the project's links to remove.
func TestLinkRemovePicksALink(t *testing.T) {
	dir, path := linkTestProject(t, linkManifest)
	d, run := pickDeps(t, dir, "2\n", PickedDeployment{}, "")
	require.NoError(t, execute(t, d, "link", "remove"))
	assertPickerTable(t, run.out.String(), "Select a link to remove", `1\s+dev\s+astro\s+dep-dev`, `2\s+stage\s+astro\s+dep-stage`)
	assert.Contains(t, run.out.String(), "removed link stage from ")
	m := loadManifest(t, path)
	assert.NotContains(t, m.Astro.Deployments, "stage")
	assert.Contains(t, m.Astro.Deployments, "dev")

	// An answer that is no row number removes nothing, as the Deployment
	// picker refuses one.
	for _, answer := range []string{"", "9\n", "dev\n", " 1\n"} {
		d, _ = pickDeps(t, dir, answer, PickedDeployment{}, "")
		err := execute(t, d, "link", "remove")
		require.ErrorIs(t, err, errInvalidLinkSelection, "answer %q", answer)
		assert.Contains(t, loadManifest(t, path).Astro.Deployments, "dev")
	}
}

// With no NAME, default asks which link, and offers none to clear it.
func TestLinkDefaultPicksALinkOrNone(t *testing.T) {
	dir, path := linkTestProject(t, linkManifest)
	d, run := pickDeps(t, dir, "2\n", PickedDeployment{}, "")
	require.NoError(t, execute(t, d, "link", "default"))
	assertPickerTable(t, run.out.String(), "Select the default link", `1\s+dev\s+astro`, `2\s+stage\s+astro`, `3\s+none\s+clear the default`)
	assert.Contains(t, run.out.String(), "set link stage as the default in ")
	m := loadManifest(t, path)
	assert.True(t, m.Astro.Deployments["stage"].Default)
	assert.False(t, m.Astro.Deployments["dev"].Default)

	d, _ = pickDeps(t, dir, "3\n", PickedDeployment{}, "")
	require.NoError(t, execute(t, d, "link", "default"))
	for name, l := range loadManifest(t, path).Astro.Deployments {
		assert.False(t, l.Default, "%s is still the default", name)
	}

	d, _ = pickDeps(t, dir, "4\n", PickedDeployment{}, "")
	require.ErrorIs(t, execute(t, d, "link", "default"), errInvalidLinkSelection)
}

// A picker row for each kind says what the link points at.
func TestLinkPickerShowsEachKind(t *testing.T) {
	body := linkManifest + "\n[tool.astro.deployments.aws]\ntarget = 'mwaa'\nenvironment = 'orders-prod'\n" +
		"\n[tool.astro.deployments.oss]\nurl = 'https://airflow.example.com'\nauth = { method = 'none' }\n"
	dir, _ := linkTestProject(t, body)
	d, run := pickDeps(t, dir, "\n", PickedDeployment{}, "")
	require.Error(t, execute(t, d, "link", "remove"))
	assertPickerTable(t, run.out.String(), "Select a link to remove",
		`1\s+aws\s+mwaa\s+orders-prod`, `3\s+oss\s+endpoint\s+https://airflow.example.com`)
}

// With no id, workspace asks which of the login's workspaces to use and links
// it on the login's host; --show, or a run that cannot ask, prints instead.
func TestLinkWorkspacePicksOrShows(t *testing.T) {
	dir, path := linkTestProject(t, linkManifest)
	d, run := pickDeps(t, dir, "", PickedDeployment{}, "ws_Z")
	d.LoginDomain = func() (string, error) { return "astronomer-dev.io", nil }
	require.NoError(t, execute(t, d, "link", "workspace"))
	assert.Equal(t, 1, run.workspacePicks)
	m := loadManifest(t, path)
	assert.Equal(t, "ws_Z", m.Astro.Workspace)
	assert.Equal(t, "astronomer-dev.io", m.Astro.Domain)
	assert.Equal(t, "ws_A", m.Astro.Deployments["dev"].Workspace, "the switch moved an inheriting link")

	for name, args := range map[string][]string{
		"--show":      {"link", "workspace", "--show"},
		"json":        {"link", "workspace", "--output", "json"},
		"no terminal": {"link", "workspace"},
	} {
		t.Run(name, func(t *testing.T) {
			sd, srun := pickDeps(t, dir, "", PickedDeployment{}, "")
			if name == "no terminal" {
				sd.Interactive = func() bool { return false }
			}
			require.NoError(t, execute(t, sd, args...))
			assert.Zero(t, srun.workspacePicks)
			assert.Contains(t, srun.out.String(), "ws_Z")
		})
	}

	d, _ = pickDeps(t, dir, "", PickedDeployment{}, "")
	d.PickWorkspace = func() (string, error) { return "", errors.New("no login") }
	require.Error(t, execute(t, d, "link", "workspace"))
	require.Error(t, execute(t, d, "link", "workspace", "--show", "--unset"))
}

// The picker is told which Deployments the project already links, by ID, so it
// leaves them out; when that leaves none, add names the links and how to
// relink one rather than offering an empty table.
func TestLinkAddLeavesOutLinkedDeployments(t *testing.T) {
	body := linkManifest + "\n[tool.astro.deployments.other]\nworkspace = 'ws_B'\ndeployment = 'dep-other'\n" +
		"\n[tool.astro.deployments.aws]\ntarget = 'mwaa'\nenvironment = 'dep-dev'\n"
	dir, _ := linkTestProject(t, body)
	d, run := pickDeps(t, dir, "", PickedDeployment{ID: "clx9", Name: "new", WorkspaceID: "ws_A"}, "")
	require.NoError(t, execute(t, d, "link", "add"))
	// Keyed by ID, astro links only: the mwaa environment that happens to be
	// spelled like a Deployment id is not one.
	assert.Equal(t, map[string]string{"dep-dev": "dev", "dep-stage": "stage", "dep-other": "other"}, run.linked)

	dir, _ = linkTestProject(t, body)
	d, run = pickDeps(t, dir, "", PickedDeployment{}, "")
	run.pickErr = ErrAllDeploymentsLinked
	err := execute(t, d, "link", "add")
	require.Error(t, err)
	assert.Equal(t, "every Deployment in workspace ws_A is already linked, as dev, stage. To relink one, run astro link add NAME --deployment <id> --replace", err.Error())
	assert.Equal(t, body, readManifest(t, dir))
}

// Naming a linked Deployment with --deployment still takes the named path: a
// NAME already linked is refused, and --replace relinks it.
func TestLinkAddALinkedDeploymentByFlag(t *testing.T) {
	dir, path := linkTestProject(t, linkManifest)
	_, _, err := runLink(t, dir, "add", "dev", "--deployment", "dep-dev")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--replace")
	_, _, err = runLink(t, dir, "add", "dev", "--deployment", "dep-dev", "--replace")
	require.NoError(t, err)
	assert.Equal(t, "dep-dev", loadManifest(t, path).Astro.Deployments["dev"].Deployment)
}
