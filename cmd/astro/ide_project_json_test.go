package astro

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	astrov1alpha1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1/mocks"
)

// What `astro ide project list`, `import` and `export` print.
//
// In text, the messages in the order a person reads them and each table cell
// under its header: what v2 printed before the commands gained --output,
// recorded byte for byte and checked here by meaning.

type ideMockOpt = func(m *astrov1alpha1_mocks.ClientWithResponsesInterface)

// ideCase is one run of an `astro ide project` command and what to check.
type ideCase struct {
	name     string
	projects []astrov1alpha1.AstroIdeProject
	more     []ideMockOpt
	files    map[string]string // what the directory holds before the run
	answers  string
	args     []string
	check    func(t *testing.T, stdout string)
	wantErr  string
	// after checks the directory once the run is over.
	after func(t *testing.T, dir string)
}

func (tc *ideCase) run(t *testing.T, extra ...string) (r tokenRun, dir string) {
	t.Helper()
	dir = ideDir(t, tc.files)
	return execIDECmd(t, ideMock(t, tc.projects, tc.more...), tc.answers, append(tc.args, extra...)...), dir
}

// exportSource is a project directory to export: a DAG, a log its
// .gitignore leaves out, and the .gitignore.
var exportSource = map[string]string{"dags/a.py": "x=1\n", "debug.log": "zz\n", ".gitignore": "*.log\n"}

// lockedETL is ETL locked by another user.
func lockedETL() astrov1alpha1.AstroIdeProject {
	etl, _ := ideProjects()
	who := "Ada Lovelace"
	etl.Lock = &astrov1alpha1.AstroIdeProjectLock{LastEditedAt: "2026-01-02T15:04:05Z", Subject: astrov1alpha1.BasicSubjectProfile{FullName: &who}}
	return etl
}

// imported checks that the run wrote the archive's files into the directory.
func imported(t *testing.T, dir string) {
	t.Helper()
	b, err := os.ReadFile(dir + "/dags/etl.py")
	require.NoError(t, err)
	assert.Equal(t, "print('etl')\n", string(b))
	b, err = os.ReadFile(dir + "/requirements.txt")
	require.NoError(t, err)
	assert.Equal(t, "pandas\n", string(b))
}

// importedNothing checks that the directory holds only what it held before.
func importedNothing(t *testing.T, dir string) {
	t.Helper()
	_, err := os.Stat(dir + "/dags/etl.py")
	assert.True(t, os.IsNotExist(err), "nothing was imported")
}

// The picker's question and its rows.
const idePickTitle = "\nPlease select the project from the list below:\n"

var idePickRows = picks(
	map[string]string{"#": "1", "PROJECT NAME": "ETL", "ID": "proj-etl"},
	map[string]string{"#": "2", "PROJECT NAME": "ML", "ID": "proj-ml"},
)

// ideTextCases are every path the three commands take in text. The JSON
// tests run the ones that ask nothing again under -o json.
func ideTextCases() []ideCase {
	etl, ml := ideProjects()
	both := []astrov1alpha1.AstroIdeProject{etl, ml}
	ro := astrov1alpha1.CreateAstroIdeSessionPermissionREADONLY
	rw := astrov1alpha1.CreateAstroIdeSessionPermissionREADWRITE
	fresh := astrov1alpha1.AstroIdeProject{Id: "proj-new", Name: "Fresh", OrganizationId: "test-org-id", WorkspaceId: curWorkspaceID, CreatedAt: ideCreated, UpdatedAt: ideCreated}
	then := func(checks ...func(t *testing.T, out string)) func(t *testing.T, out string) {
		return func(t *testing.T, out string) {
			for _, c := range checks {
				c(t, out)
			}
		}
	}
	return []ideCase{
		{
			name: "list", projects: both, args: []string{"project", "list"},
			check: listsRows("NAME", map[string]string{"NAME": "ETL", "ID": "proj-etl"}, map[string]string{"NAME": "ML", "ID": "proj-ml"}),
		},
		{name: "list with no projects", args: []string{"project", "list"}, check: listsRows("NAME")},
		{name: "list refused by the API", projects: both, more: []ideMockOpt{failsList(403)}, args: []string{"project", "list"}, check: says(""), wantErr: "no IDE for you"},
		// The success line of an import has always said "exported".
		{
			name: "import by id", projects: both, more: []ideMockOpt{opensSession(ro)}, args: []string{"project", "import", "-p", "proj-etl"},
			check: says("Successfully exported project from ETL\n"), after: imported,
		},
		{
			name: "import from a session", projects: both, args: []string{"project", "import", "-p", "proj-etl", "-s", "sess-x"},
			check: says("Successfully exported project from ETL\n"), after: imported,
		},
		{
			name: "import into a directory that is not empty, confirmed", projects: both, more: []ideMockOpt{opensSession(ro)},
			files: map[string]string{"README.md": "hi\n"}, answers: "y\n", args: []string{"project", "import", "-p", "proj-etl"},
			check: says("Current directory is not empty. Do you want to import the project here? ", " (y/n) ", "Successfully exported project from ETL\n"), after: imported,
		},
		{
			name: "import into a directory that is not empty, declined", projects: both,
			files: map[string]string{"README.md": "hi\n"}, answers: "n\n", args: []string{"project", "import", "-p", "proj-etl"},
			check: then(says("Current directory is not empty.", " (y/n) "), lacks("Successfully")), wantErr: "import canceled by user", after: importedNothing,
		},
		{
			name: "import into a directory that is not empty --yes", projects: both, more: []ideMockOpt{opensSession(ro)},
			files: map[string]string{"README.md": "hi\n"}, args: []string{"project", "import", "-p", "proj-etl", "--yes"},
			check: then(says("Successfully exported project from ETL\n"), lacks("Current directory is not empty")), after: imported,
		},
		{
			name: "import through the picker", projects: both, more: []ideMockOpt{opensSession(ro)}, answers: "2\n", args: []string{"project", "import"},
			check: then(idePickRows, says(idePickTitle, "\n> ", "Successfully exported project from ML\n")), after: imported,
		},
		{
			name: "import through the picker, answered with no row", projects: both, answers: "9\n", args: []string{"project", "import"},
			check: then(idePickRows, says(idePickTitle, "\n> "), lacks("Successfully")), wantErr: "invalid project selection", after: importedNothing,
		},
		{
			name: "import of the only project", projects: []astrov1alpha1.AstroIdeProject{etl}, more: []ideMockOpt{opensSession(ro)}, args: []string{"project", "import"},
			check: plain(says("Only one Project was found. Using the following Project by default: \n", "\n Project Name: ETL", "\n Project ID: proj-etl\n", "Successfully exported project from ETL\n")),
			after: imported,
		},
		{name: "import with no projects", args: []string{"project", "import"}, check: says(""), wantErr: "no Astro IDE projects found in workspace", after: importedNothing},
		{
			name: "export by id", projects: both, more: []ideMockOpt{opensSession(rw), acceptsUpload}, files: exportSource,
			args: []string{"project", "export", "-p", "proj-etl"}, check: says("Successfully exported project to ETL\n"),
		},
		// A project that cannot be read back afterwards is named by its ID.
		{
			name: "export to a project that cannot be read back", projects: both, more: []ideMockOpt{opensSession(rw), acceptsUpload, cannotRead("proj-gone")}, files: exportSource,
			args: []string{"project", "export", "-p", "proj-gone"}, check: says("Successfully exported project to proj-gone\n"),
		},
		{
			name: "export to a locked project", projects: []astrov1alpha1.AstroIdeProject{lockedETL(), ml}, more: []ideMockOpt{opensSession(ro)}, files: exportSource,
			args: []string{"project", "export", "-p", "proj-etl"}, check: says(""),
			wantErr: "project is locked by user Ada Lovelace and last edited at January 2, 2026 at 3:04 PM. Use --force flag to overwrite the existing project lock",
		},
		{
			name: "export to a locked project --force", projects: []astrov1alpha1.AstroIdeProject{lockedETL(), ml}, more: []ideMockOpt{opensSession(ro), acceptsUpload}, files: exportSource,
			args: []string{"project", "export", "-p", "proj-etl", "--force"}, check: says("Successfully exported project to ETL\n"),
		},
		{
			name: "export to a project picked", projects: both, more: []ideMockOpt{opensSession(rw), acceptsUpload}, files: exportSource,
			answers: "n\n2\n", args: []string{"project", "export"},
			check: then(idePickRows, says("Do you want to create a new project? (y/n)\n", "\n> ", idePickTitle, "\n> ", "Successfully exported project to ML\n")),
		},
		{
			name: "export to a new project", projects: both, more: []ideMockOpt{opensSession(rw), acceptsUpload, createsProject(fresh)}, files: exportSource,
			answers: "y\nFresh\n", args: []string{"project", "export"},
			check: says("Do you want to create a new project? (y/n)\n", "\n> ", "Enter project name:\n", "\n> ",
				"Successfully created project 'Fresh' in workspace 'Development'\n", "Successfully exported project to Fresh\n"),
		},
		// A project the run created outlives the failure after it, and the
		// error names it.
		{
			name: "export to a new project, the upload failing", projects: both, more: []ideMockOpt{opensSession(rw), failsUpload, createsProject(fresh)}, files: exportSource,
			answers: "y\nFresh\n", args: []string{"project", "export"},
			check:   then(says("Successfully created project 'Fresh' in workspace 'Development'\n"), lacks("Successfully exported")),
			wantErr: "created the Astro IDE project proj-new, but the export to it failed: upload broke",
		},
		{
			name: "export to a new project the API does not return", projects: both, more: []ideMockOpt{createsNothing}, files: exportSource,
			answers: "y\nFresh\n", args: []string{"project", "export"},
			check:   then(says("Enter project name:\n"), lacks("Successfully")),
			wantErr: "failed to create project: the API did not return the project",
		},
		{
			name: "import, the export failing", projects: both, more: []ideMockOpt{opensSession(ro), failsDownload}, args: []string{"project", "import", "-p", "proj-etl"},
			check: says(""), wantErr: "export broke", after: importedNothing,
		},
		{
			name: "export --force to a project picked", projects: both, more: []ideMockOpt{opensSession(rw), acceptsUpload}, files: exportSource,
			answers: "1\n", args: []string{"project", "export", "--force"},
			check: then(idePickRows, says(idePickTitle, "\n> ", "Successfully exported project to ETL\n"), lacks("create a new project")),
		},
	}
}

// What each command prints in text.
func TestIDEProjectText(t *testing.T) {
	for _, tc := range ideTextCases() {
		t.Run(tc.name, func(t *testing.T) {
			r, dir := tc.run(t)
			if tc.wantErr != "" {
				require.Error(t, r.err)
				assert.Equal(t, tc.wantErr, r.err.Error())
				assert.Equal(t, cliout.ExitFailure, r.code)
			} else {
				require.NoError(t, r.err)
				assert.Equal(t, 0, r.code)
			}
			tc.check(t, r.stdout)
			assert.Empty(t, r.stderr)
			if tc.after != nil {
				tc.after(t, dir)
			}
		})
	}
}

// ideJSON is a project as the list publishes it.
func ideJSON(p *astrov1alpha1.AstroIdeProject) map[string]any {
	out := map[string]any{
		"id": p.Id, "name": p.Name, "workspace_id": p.WorkspaceId, "organization_id": p.OrganizationId,
		"created_at": p.CreatedAt.Format(time.RFC3339), "updated_at": p.UpdatedAt.Format(time.RFC3339),
	}
	if p.Description != nil {
		out["description"] = *p.Description
	}
	if p.Url != nil {
		out["url"] = *p.Url
	}
	return out
}

// sameDir reads a published directory the way the test's own is spelled:
// a temporary directory on macOS is reached through a symlink.
func sameDir(t *testing.T, want, got string) {
	t.Helper()
	w, err := filepath.EvalSymlinks(want)
	require.NoError(t, err)
	g, err := filepath.EvalSymlinks(got)
	require.NoError(t, err)
	assert.Equal(t, w, g)
}

// movedJSON checks an import or an export: every key, with the directory
// read as sameDir does.
func movedJSON(want map[string]any) func(t *testing.T, stdout, dir string) {
	return func(t *testing.T, stdout, dir string) {
		var got map[string]any
		decodeOne(t, stdout, &got)
		gotDir, _ := got["directory"].(string)
		sameDir(t, dir, gotDir)
		delete(got, "directory")
		assert.Equal(t, want, got)
	}
}

// What each command publishes under --output json, and that it publishes
// nothing else: stdout is the one object, the exit is 0, and stderr holds
// only the notes a person would have read (none, but for the "only one
// Project" one).
func TestIDEProjectJSON(t *testing.T) {
	etl, ml := ideProjects()
	both := []astrov1alpha1.AstroIdeProject{etl, ml}
	ro := astrov1alpha1.CreateAstroIdeSessionPermissionREADONLY
	rw := astrov1alpha1.CreateAstroIdeSessionPermissionREADWRITE
	url := "https://ide.example/proj-ml"
	mlURL := ml
	mlURL.Url = &url
	// Two files and 20 bytes come out of the IDE's archive; a DAG and the
	// .gitignore, 10 bytes, go into it, the .gitignore'd log left behind.
	importedETL := map[string]any{"project_id": "proj-etl", "project_name": "ETL", "session_id": ideSessionRO, "files": float64(2), "bytes": float64(20), "action": "imported"}
	exportedETL := map[string]any{"project_id": "proj-etl", "project_name": "ETL", "project_created": false, "files": float64(2), "bytes": float64(10), "action": "exported"}

	cases := []struct {
		ideCase
		check  func(t *testing.T, stdout, dir string)
		stderr []string
	}{
		{
			ideCase: ideCase{name: "list", projects: both, args: []string{"project", "list"}},
			check: func(t *testing.T, stdout, _ string) {
				var got map[string]any
				decodeOne(t, stdout, &got)
				assert.Equal(t, map[string]any{"projects": []any{ideJSON(&etl), ideJSON(&ml)}}, got)
			},
		},
		{
			ideCase: ideCase{name: "list with a URL", projects: []astrov1alpha1.AstroIdeProject{mlURL}, args: []string{"project", "list"}},
			check: func(t *testing.T, stdout, _ string) {
				var got map[string]any
				decodeOne(t, stdout, &got)
				assert.Equal(t, map[string]any{"projects": []any{ideJSON(&mlURL)}}, got)
			},
		},
		{
			ideCase: ideCase{name: "list with no projects", args: []string{"project", "list"}},
			check: func(t *testing.T, stdout, _ string) {
				var got map[string]any
				fields := decodeOne(t, stdout, &got)
				assert.JSONEq(t, `[]`, string(fields["projects"]), "an empty array, not null and not a missing key")
			},
		},
		{
			ideCase: ideCase{name: "import by id", projects: both, more: []ideMockOpt{opensSession(ro)}, args: []string{"project", "import", "-p", "proj-etl"}, after: imported},
			check:   movedJSON(importedETL),
		},
		{
			ideCase: ideCase{name: "import from a session", projects: both, args: []string{"project", "import", "-p", "proj-etl", "-s", "sess-x"}, after: imported},
			check:   movedJSON(map[string]any{"project_id": "proj-etl", "project_name": "ETL", "session_id": "sess-x", "files": float64(2), "bytes": float64(20), "action": "imported"}),
		},
		{
			ideCase: ideCase{
				name: "import into a directory that is not empty --yes", projects: both, more: []ideMockOpt{opensSession(ro)},
				files: map[string]string{"README.md": "hi\n"}, args: []string{"project", "import", "-p", "proj-etl", "--yes"}, after: imported,
			},
			check: movedJSON(importedETL),
		},
		// The only project is used without asking; the note saying so is
		// for a person, so it goes to stderr.
		{
			ideCase: ideCase{name: "import of the only project", projects: []astrov1alpha1.AstroIdeProject{etl}, more: []ideMockOpt{opensSession(ro)}, args: []string{"project", "import"}, after: imported},
			check:   movedJSON(importedETL),
			stderr:  []string{"Only one Project was found.", "Project ID: ", "proj-etl"},
		},
		{
			ideCase: ideCase{name: "export by id", projects: both, more: []ideMockOpt{opensSession(rw), acceptsUpload}, files: exportSource, args: []string{"project", "export", "-p", "proj-etl"}},
			check:   movedJSON(exportedETL),
		},
		// The URL is published instead of opened.
		{
			ideCase: ideCase{name: "export to a project with a URL", projects: []astrov1alpha1.AstroIdeProject{etl, mlURL}, more: []ideMockOpt{opensSession(rw), acceptsUpload}, files: exportSource, args: []string{"project", "export", "-p", "proj-ml"}},
			check:   movedJSON(map[string]any{"project_id": "proj-ml", "project_name": "ML", "project_created": false, "url": url, "files": float64(2), "bytes": float64(10), "action": "exported"}),
		},
		// With no name to give, the result gives none.
		{
			ideCase: ideCase{name: "export to a project that cannot be read back", projects: both, more: []ideMockOpt{opensSession(rw), acceptsUpload, cannotRead("proj-gone")}, files: exportSource, args: []string{"project", "export", "-p", "proj-gone"}},
			check:   movedJSON(map[string]any{"project_id": "proj-gone", "project_created": false, "files": float64(2), "bytes": float64(10), "action": "exported"}),
		},
		{
			ideCase: ideCase{name: "export to a locked project --force", projects: []astrov1alpha1.AstroIdeProject{lockedETL(), ml}, more: []ideMockOpt{opensSession(ro), acceptsUpload}, files: exportSource, args: []string{"project", "export", "-p", "proj-etl", "--force"}},
			check:   movedJSON(exportedETL),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, dir := tc.run(t, "-o", "json")
			require.NoError(t, r.err)
			assert.Equal(t, 0, r.code)
			tc.check(t, r.stdout, dir)
			if tc.stderr == nil {
				assert.Empty(t, r.stderr)
			} else {
				requireInOrder(t, sgr.ReplaceAllString(r.stderr, ""), tc.stderr...)
			}
			if tc.after != nil {
				tc.after(t, dir)
			}
		})
	}
}

// A failure under --output json is the error object alone.
func TestIDEProjectJSONFailures(t *testing.T) {
	_, ml := ideProjects()
	ro := astrov1alpha1.CreateAstroIdeSessionPermissionREADONLY
	for _, tc := range []ideCase{
		{name: "list refused by the API", more: []ideMockOpt{failsList(403)}, args: []string{"project", "list"}, wantErr: "no IDE for you"},
		{name: "import with no projects", args: []string{"project", "import"}, wantErr: "no Astro IDE projects found in workspace", after: importedNothing},
		{
			name: "export to a locked project", projects: []astrov1alpha1.AstroIdeProject{lockedETL(), ml}, more: []ideMockOpt{opensSession(ro)}, files: exportSource,
			args: []string{"project", "export", "-p", "proj-etl"}, wantErr: "project is locked by user Ada Lovelace",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, dir := tc.run(t, "-o", "json")
			require.Error(t, r.err)
			assert.Equal(t, cliout.ExitFailure, r.code)
			assertOnlyTheError(t, "json", r, tc.wantErr)
			if tc.after != nil {
				tc.after(t, dir)
			}
		})
	}
}

// Under --output json a question these commands would ask fails as
// input_required, naming what answers it, with that object as the whole of
// stdout: no picker table, no prompt, no line introducing the question. An
// answer waits on stdin, so a prompt that did read would go on, and the
// client mocks no session, upload or create, so a refused question that went
// on to act would fail on the mock.
func TestIDEProjectJSONNeverAsks(t *testing.T) {
	etl, ml := ideProjects()
	both := []astrov1alpha1.AstroIdeProject{etl, ml}
	notEmpty := map[string]string{"README.md": "hi\n"}
	for _, tc := range []struct {
		ideCase
		answered string
	}{
		{ideCase{name: "import naming no project", projects: both, args: []string{"project", "import"}}, "pass --project-id"},
		{ideCase{name: "import into a directory that is not empty", projects: both, files: notEmpty, args: []string{"project", "import", "-p", "proj-etl"}}, "pass --yes"},
		{ideCase{name: "export naming no project", projects: both, files: exportSource, args: []string{"project", "export"}}, "pass --project-id"},
		{ideCase{name: "export --force naming no project", projects: both, files: exportSource, args: []string{"project", "export", "--force"}}, "pass --project-id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.answers = "y\n1\nFresh\n"
			r, dir := tc.run(t, "-o", "json")
			require.Error(t, r.err)
			assert.Equal(t, cliout.ExitFailure, r.code)
			var got errorJSON
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, string(cliout.KindInputRequired), got.Kind)
			assert.Equal(t, cliout.ExitFailure, got.Code, "the code it reports is the one it exits with")
			assert.Contains(t, got.Error, tc.answered)
			assert.Empty(t, r.stderr)
			importedNothing(t, dir)
		})
	}
}

// The non-empty-directory confirmation names the directory the import
// writes into.
func TestIDEProjectImportConfirmationNamesTheDirectory(t *testing.T) {
	etl, ml := ideProjects()
	tc := ideCase{projects: []astrov1alpha1.AstroIdeProject{etl, ml}, files: map[string]string{"README.md": "hi\n"}, answers: "n\n", args: []string{"project", "import", "-p", "proj-etl"}}
	r, dir := tc.run(t)
	require.Error(t, r.err)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	sameDir(t, dir, cwd)
	assert.Contains(t, r.stdout, "Do you want to import the project here? "+cwd+" (y/n) ")
}

// -o takes text or json, and anything else is a usage error before any
// request.
func TestIDEProjectRefusesAnUnknownFormat(t *testing.T) {
	for _, args := range [][]string{
		{"project", "list", "-o", "yaml"},
		{"project", "import", "-p", "proj-etl", "-o", "yaml"},
		{"project", "export", "-p", "proj-etl", "-o", "yaml"},
	} {
		t.Run(args[1], func(t *testing.T) {
			m := ideMock(t, nil)
			ideDir(t, nil)
			r := execIDECmd(t, m, "", args...)
			require.Error(t, r.err)
			assert.Equal(t, cliout.ExitUsage, r.code)
			assert.Empty(t, m.Calls)
		})
	}
}
