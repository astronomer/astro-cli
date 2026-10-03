package scaffold

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// Every fixture here is written by hand, never by the writer under test, so
// what the writer produces is compared against a file it did not make.

// linkFixture is a project a person wrote, with a comment every write must
// leave in place.
const linkFixture = `[project]
name = 'demo'
version = '0.1.0'
requires-python = '>=3.10'
dependencies = ['apache-airflow==3.1.*']

# A comment the user wrote, which must survive every write.
[tool.astro]
`

const userComment = "# A comment the user wrote, which must survive every write."

// inheritingFixture sets both project-level defaults a link can inherit.
const inheritingFixture = `[project]
name = 'demo'
version = '0.1.0'
dependencies = ['apache-airflow==3.1.*']

[tool.astro]
target = 'astro'
workspace = 'ws-default'
domain = 'astronomer.io'
`

// linkProject writes body as the project's manifest and returns its directory
// and the manifest's path.
func linkProject(t *testing.T, body string) (dir, path string) {
	t.Helper()
	return writeEditFixture(t, body, 0o644)
}

// loadLinks parses the manifest the way every reader does.
func loadLinks(t *testing.T, path string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.Load(path)
	require.NoError(t, err)
	return m
}

// onlyLink is the manifest's one link, which the test requires there to be.
func onlyLink(t *testing.T, path string) (string, manifest.Link) {
	t.Helper()
	m := loadLinks(t, path)
	require.Len(t, m.Astro.Deployments, 1)
	for name := range m.Astro.Deployments {
		return name, m.Astro.Deployments[name]
	}
	return "", manifest.Link{}
}

// ownKeys is the target and workspace the named link sets itself, read from the
// raw TOML: the parsed manifest folds the defaults in, so it cannot tell an
// inherited value from a written one.
func ownKeys(t *testing.T, path, name string) map[string]any {
	t.Helper()
	ed, err := tomledit.NewSurgical([]byte(readFile(t, path)))
	require.NoError(t, err)
	_, ok := ed.Get(linkKey(name))
	require.True(t, ok, "no link %q in:\n%s", name, readFile(t, path))
	own := map[string]any{}
	for _, key := range []string{"target", "workspace"} {
		if v, ok := ed.Get(append(linkKey(name), key)); ok {
			own[key] = v
		}
	}
	return own
}

// setWorkspace is SetWorkspaceLink for a test that does not look at which links
// it pinned.
func setWorkspace(t *testing.T, dir, id, domain string) error {
	t.Helper()
	_, err := SetWorkspaceLink(dir, nil, id, domain, "")
	return err
}

func saveLink(t *testing.T, dir string, l Link) { //nolint:gocritic // hugeParam: a test helper taking the value SaveLink does
	t.Helper()
	require.NoError(t, SaveLink(dir, nil, l))
}

func endpointLink(a *manifest.Auth) Link {
	return Link{Name: "oss", Kind: manifest.KindEndpoint, URL: "https://airflow.example.com", Auth: *a}
}

func composerLink(name string) Link {
	return Link{
		Name: name, Kind: manifest.KindComposer, Environment: name,
		TargetProject: "acme-data", TargetLocation: "us-central1",
	}
}

// Each kind round-trips with its own coordinates and comes back with the kind
// the parser derives, since nothing writes a kind: a link that came back as the
// wrong one would mean its coordinates went under the wrong keys.
func TestSaveLinkRoundTripsEachKind(t *testing.T) {
	for _, tc := range []Link{
		{Name: "prod", Kind: manifest.KindAstro, Workspace: "Data", Deployment: "clx123"},
		{Name: "aws-prod", Kind: manifest.KindMWAA, Environment: "my-mwaa-env"},
		composerLink("gcp-prod"),
		endpointLink(&manifest.Auth{Method: manifest.AuthToken, TokenEnv: "AIRFLOW_TOKEN"}),
	} {
		t.Run(string(tc.Kind), func(t *testing.T) {
			dir, path := linkProject(t, linkFixture)
			saveLink(t, dir, tc)
			name, got := onlyLink(t, path)
			assert.Equal(t, tc.Name, name)
			assert.Equal(t, tc.Kind, got.Kind())
			assert.Equal(t, tc.Workspace, got.Workspace)
			assert.Equal(t, tc.Deployment, got.Deployment)
			assert.Equal(t, tc.Environment, got.Environment)
			assert.Equal(t, tc.URL, got.URL)
		})
	}
}

// A kind with a default method gets no auth table, and reads back with the
// method its kind implies. One that overrides the default gets its table.
func TestSaveLinkWritesAnAuthTableOnlyWhenItSaysSomething(t *testing.T) {
	dir, path := linkProject(t, linkFixture)
	saveLink(t, dir, Link{Name: "aws-prod", Kind: manifest.KindMWAA, Environment: "env", Auth: manifest.Auth{Method: manifest.AuthAWS}})
	assert.NotContains(t, readFile(t, path), "auth")
	_, got := onlyLink(t, path)
	assert.Equal(t, manifest.AuthAWS, got.Auth.Method)

	saveLink(t, dir, Link{Name: "aws-prod", Kind: manifest.KindMWAA, Environment: "env", Auth: manifest.Auth{Method: manifest.AuthToken, TokenEnv: "AF_TOKEN"}})
	_, got = onlyLink(t, path)
	assert.Equal(t, manifest.Auth{Method: manifest.AuthToken, TokenEnv: "AF_TOKEN"}, got.Auth)
}

// A save leaves the rest of the manifest as the person wrote it.
func TestSaveLinkLeavesTheRestOfTheManifestAlone(t *testing.T) {
	dir, path := linkProject(t, linkFixture)
	saveLink(t, dir, Link{Name: "prod", Kind: manifest.KindAstro, Workspace: "Data", Deployment: "clx123"})
	out := readFile(t, path)
	assert.Contains(t, out, userComment)
	assert.Contains(t, out, "'apache-airflow==3.1.*'")
	assert.True(t, strings.HasPrefix(out, linkFixture), "the lines before the new link changed:\n%s", out)
}

// A link the form-level check catches is refused with ErrInvalidLink and
// nothing is written.
func TestSaveLinkRefusesAnInvalidLink(t *testing.T) {
	for _, tc := range []struct {
		name string
		link Link
		want string
	}{
		{"no name", Link{Name: "  ", Kind: manifest.KindAstro, Workspace: "W", Deployment: "d"}, "needs a name"},
		{"the reserved name", Link{Name: manifest.ReservedLinkName, Kind: manifest.KindMWAA, Environment: "env"}, "reserved"},
		{"an astro link with no deployment", Link{Name: "prod", Kind: manifest.KindAstro, Workspace: "W"}, "needs a deployment"},
		{"an mwaa link with no environment", Link{Name: "aws", Kind: manifest.KindMWAA}, "needs an environment"},
		{"an endpoint with no url", Link{Name: "oss", Kind: manifest.KindEndpoint, Auth: manifest.Auth{Method: manifest.AuthNone}}, "needs a url"},
		{"an endpoint with no auth", Link{Name: "oss", Kind: manifest.KindEndpoint, URL: "https://airflow.example.com"}, "how its Airflow checks callers"},
		{"an unknown kind", Link{Name: "x", Kind: "sorcery"}, "unknown link kind"},
		{"token with no variable", endpointLink(&manifest.Auth{Method: manifest.AuthToken}), "holding the token"},
		{"basic with only a username", endpointLink(&manifest.Auth{Method: manifest.AuthBasic, UsernameEnv: "AF_USER"}), "both a username and a password"},
		{"exec with no command", endpointLink(&manifest.Auth{Method: manifest.AuthExec}), "a command to run"},
		{"an unknown method", endpointLink(&manifest.Auth{Method: "sorcery"}), "unknown auth method"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, path := linkProject(t, linkFixture)
			err := SaveLink(dir, nil, tc.link)
			require.ErrorIs(t, err, ErrInvalidLink)
			assert.Contains(t, err.Error(), tc.want)
			assert.Equal(t, linkFixture, readFile(t, path))
		})
	}
}

// A credential pasted where an env var name belongs is refused, and the error
// does not quote it: the message may land in a terminal or a CI log, and the
// manifest is committed.
func TestSaveLinkRefusesAPastedCredentialWithoutEchoingIt(t *testing.T) {
	const secret = "eyJhbGciOiJIUzI1NiJ9.s3cr3t"
	for _, a := range []manifest.Auth{
		{Method: manifest.AuthToken, TokenEnv: secret},
		{Method: manifest.AuthBasic, UsernameEnv: "AF_USER", PasswordEnv: secret},
		{Method: manifest.AuthAirflowToken, ClientIDEnv: "AF_ID", ClientSecretEnv: secret},
	} {
		dir, path := linkProject(t, linkFixture)
		err := SaveLink(dir, nil, endpointLink(&a))
		require.ErrorIs(t, err, ErrInvalidLink)
		assert.Contains(t, err.Error(), "NAME of an environment variable")
		assert.NotContains(t, err.Error(), "s3cr3t", "the refusal quoted the credential back")
		assert.NotContains(t, readFile(t, path), "s3cr3t")
	}
}

// airflow-token takes exactly one whole credential pair.
func TestSaveLinkAirflowTokenTakesExactlyOnePair(t *testing.T) {
	for _, tc := range []struct {
		name string
		auth manifest.Auth
		ok   bool
	}{
		{"client pair", manifest.Auth{Method: manifest.AuthAirflowToken, ClientIDEnv: "AF_CLIENT_ID", ClientSecretEnv: "AF_CLIENT_SECRET"}, true},
		{"user pair", manifest.Auth{Method: manifest.AuthAirflowToken, UsernameEnv: "AF_USER", PasswordEnv: "AF_PASSWORD"}, true},
		{"half a pair", manifest.Auth{Method: manifest.AuthAirflowToken, ClientIDEnv: "AF_CLIENT_ID"}, false},
		{"both pairs", manifest.Auth{
			Method:      manifest.AuthAirflowToken,
			ClientIDEnv: "AF_CLIENT_ID", ClientSecretEnv: "AF_CLIENT_SECRET",
			UsernameEnv: "AF_USER", PasswordEnv: "AF_PASSWORD",
		}, false},
		{"no pair", manifest.Auth{Method: manifest.AuthAirflowToken}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, path := linkProject(t, linkFixture)
			err := SaveLink(dir, nil, endpointLink(&tc.auth))
			if tc.ok {
				require.NoError(t, err)
				_, got := onlyLink(t, path)
				assert.Equal(t, tc.auth, got.Auth)
				return
			}
			require.ErrorIs(t, err, ErrInvalidLink)
		})
	}
}

// An exec link keeps its command through a read and a re-save, the way an edit
// of another field rewrites it.
func TestSaveLinkKeepsAnExecCommand(t *testing.T) {
	dir, path := linkProject(t, linkFixture)
	command := []string{"gcloud", "auth", "print-identity-token"}
	saveLink(t, dir, endpointLink(&manifest.Auth{Method: manifest.AuthExec, Command: command}))
	_, got := onlyLink(t, path)
	require.Equal(t, command, got.Auth.Command)

	again := endpointLink(&got.Auth)
	again.URL = "https://airflow2.example.com"
	saveLink(t, dir, again)
	_, got = onlyLink(t, path)
	assert.Equal(t, command, got.Auth.Command)
	assert.Equal(t, "https://airflow2.example.com", got.URL)
}

// Switching method drops the old method's fields, which the parser would
// refuse beside the new one.
func TestSaveLinkDropsTheOldMethodsFields(t *testing.T) {
	dir, path := linkProject(t, linkFixture)
	saveLink(t, dir, endpointLink(&manifest.Auth{Method: manifest.AuthToken, TokenEnv: "AF_TOKEN", UsernameEnv: "AF_USER", PasswordEnv: "AF_PASSWORD"}))
	out := readFile(t, path)
	assert.NotContains(t, out, "username-env")
	assert.NotContains(t, out, "password-env")
	_, got := onlyLink(t, path)
	assert.Equal(t, manifest.Auth{Method: manifest.AuthToken, TokenEnv: "AF_TOKEN"}, got.Auth)
}

// A link only the parser can refuse is not written, and the manifest still
// loads: the parser refuses the whole file over one bad link.
func TestSaveLinkRefusesWhatTheParserRefuses(t *testing.T) {
	for _, tc := range []struct {
		name string
		link Link
		want string
	}{
		{"a url with no scheme", Link{Name: "oss", Kind: manifest.KindEndpoint, URL: "airflow.example.com", Auth: manifest.Auth{Method: manifest.AuthNone}}, "has no scheme"},
		{"a url with no host", Link{Name: "oss", Kind: manifest.KindEndpoint, URL: "https://", Auth: manifest.Auth{Method: manifest.AuthNone}}, "names no host"},
		{"an empty argv element", endpointLink(&manifest.Auth{Method: manifest.AuthExec, Command: []string{"tool", "", "end"}}), "non-empty string"},
		{"an astro link with no workspace anywhere", Link{Name: "prod", Kind: manifest.KindAstro, Deployment: "clx1"}, "workspace"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, path := linkProject(t, linkFixture)
			err := SaveLink(dir, nil, tc.link)
			require.ErrorIs(t, err, ErrEditRefused)
			assert.Contains(t, err.Error(), tc.want)
			assert.Equal(t, linkFixture, readFile(t, path))
		})
	}
}

// An astro link takes the project's workspace when it names none.
func TestSaveLinkAnAstroLinkTakesTheProjectWorkspace(t *testing.T) {
	dir, path := linkProject(t, linkFixture+"workspace = 'Data'\n")
	saveLink(t, dir, Link{Name: "prod", Kind: manifest.KindAstro, Deployment: "clx1"})
	_, got := onlyLink(t, path)
	assert.Equal(t, "Data", got.Workspace)
	assert.NotContains(t, ownKeys(t, path, "prod"), "workspace")
}

// A link a person wrote under its own header can be saved over: tomledit
// refuses to Set over a table.
func TestSaveLinkReplacesAHeaderTable(t *testing.T) {
	dir, path := linkProject(t, linkFixture+"workspace = 'Data'\n\n[tool.astro.deployments.prod]\ndeployment = 'clx123'\n")
	saveLink(t, dir, Link{Name: "prod", Kind: manifest.KindAstro, Workspace: "Data", Deployment: "clx999"})
	_, got := onlyLink(t, path)
	assert.Equal(t, "clx999", got.Deployment)
	assert.Contains(t, readFile(t, path), userComment)
}

// A re-save keeps the default flag, which belongs to SetDefaultLink.
func TestSaveLinkKeepsTheDefault(t *testing.T) {
	for name, body := range map[string]string{
		"header": linkFixture + "\n[tool.astro.deployments.prod]\nworkspace = 'W'\ndeployment = 'clx1'\ndefault = true\n",
		"inline": linkFixture + "\n[tool.astro.deployments]\nprod = { workspace = 'W', deployment = 'clx1', default = true }\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir, path := linkProject(t, body)
			saveLink(t, dir, Link{Name: "prod", Kind: manifest.KindAstro, Workspace: "W", Deployment: "clx2"})
			_, got := onlyLink(t, path)
			assert.True(t, got.Default, "a re-save dropped the default")
			assert.Equal(t, "clx2", got.Deployment)
		})
	}
}

// The name is trimmed before it becomes a key.
func TestSaveLinkTrimsTheName(t *testing.T) {
	dir, path := linkProject(t, linkFixture)
	saveLink(t, dir, Link{Name: "  prod  ", Kind: manifest.KindAstro, Workspace: "W", Deployment: "clx1"})
	name, _ := onlyLink(t, path)
	assert.Equal(t, "prod", name)
}

// SaveLink never creates a manifest or turns someone else's Python project
// into an Astro one, and does not edit one that does not parse.
func TestSaveLinkRefusesAManifestItCannotEdit(t *testing.T) {
	link := Link{Name: "prod", Kind: manifest.KindAstro, Workspace: "W", Deployment: "clx1"}

	missing := t.TempDir()
	require.ErrorIs(t, SaveLink(missing, nil, link), manifest.ErrNotFound)
	_, err := os.Stat(filepath.Join(missing, manifest.Marker))
	assert.True(t, os.IsNotExist(err), "a manifest was created")

	const plain = "[project]\nname = 'plain'\n\n[tool.ruff]\nline-length = 100\n"
	dir, path := linkProject(t, plain)
	require.ErrorIs(t, SaveLink(dir, nil, link), manifest.ErrNoAstroSection)
	assert.Equal(t, plain, readFile(t, path))

	const broken = "[project\nname = "
	dir, path = linkProject(t, broken)
	require.Error(t, SaveLink(dir, nil, link))
	assert.Equal(t, broken, readFile(t, path))
}

// A link whose target and workspace match the project defaults writes neither,
// so it follows [tool.astro] when those change.
func TestSaveLinkMatchingTheDefaultsWritesNeither(t *testing.T) {
	dir, path := linkProject(t, inheritingFixture)
	saveLink(t, dir, Link{Name: "prod", Kind: manifest.KindAstro, Workspace: "ws-default", Deployment: "clx123"})
	assert.Empty(t, ownKeys(t, path, "prod"), "the link repeated [tool.astro]:\n%s", readFile(t, path))

	moved := strings.Replace(readFile(t, path), "workspace = 'ws-default'", "workspace = 'ws-moved'", 1)
	require.NoError(t, os.WriteFile(path, []byte(moved), 0o600))
	_, got := onlyLink(t, path)
	assert.Equal(t, "ws-moved", got.Workspace)
}

// A link in another workspace than the project's states its own.
func TestSaveLinkInAnotherWorkspaceWritesIt(t *testing.T) {
	dir, path := linkProject(t, inheritingFixture)
	saveLink(t, dir, Link{Name: "prod", Kind: manifest.KindAstro, Workspace: "ws-other", Deployment: "clx123"})
	assert.Equal(t, "ws-other", ownKeys(t, path, "prod")["workspace"])
}

// An endpoint's target is astro, which is also what a link with no target
// inherits in these projects, so none is written. In a project whose target is
// another platform it is written, or the endpoint would conflict with it.
func TestSaveLinkAnEndpointWritesATargetOnlyWhereItDiffers(t *testing.T) {
	for name, tc := range map[string]struct {
		body   string
		target any
	}{
		"project target astro": {inheritingFixture, nil},
		"no project target":    {strings.Replace(inheritingFixture, "target = 'astro'\n", "", 1), nil},
		"project target mwaa":  {strings.Replace(inheritingFixture, "target = 'astro'", "target = 'mwaa'", 1), "astro"},
	} {
		t.Run(name, func(t *testing.T) {
			dir, path := linkProject(t, tc.body)
			saveLink(t, dir, endpointLink(&manifest.Auth{Method: manifest.AuthNone}))
			assert.Equal(t, tc.target, ownKeys(t, path, "oss")["target"])
			_, got := onlyLink(t, path)
			assert.Equal(t, manifest.KindEndpoint, got.Kind())
		})
	}
}

// A link whose kind is not the inherited target states it.
func TestSaveLinkOffTheDefaultTargetWritesIt(t *testing.T) {
	dir, path := linkProject(t, inheritingFixture)
	saveLink(t, dir, Link{Name: "aws", Kind: manifest.KindMWAA, Environment: "env"})
	assert.Equal(t, "mwaa", ownKeys(t, path, "aws")["target"])

	for _, l := range []Link{
		{Name: "prod", Kind: manifest.KindAstro, Workspace: "W", Deployment: "clx1"},
		composerLink("gcp"),
	} {
		dir, path := linkProject(t, linkFixture+"target = 'mwaa'\n")
		saveLink(t, dir, l)
		_, got := onlyLink(t, path)
		assert.Equal(t, l.Kind, got.Kind(), "the project target was inherited")
	}
}

// A link that already sets its own target and workspace keeps them, even where
// they equal the defaults, in both spellings a person writes.
func TestSaveLinkKeepsKeysTheLinkPins(t *testing.T) {
	for name, links := range map[string]string{
		"inline": "\n[tool.astro.deployments]\n" +
			"prod = {deployment = 'clx123', workspace = 'ws-default', target = 'astro'}\n",
		"header": "\n[tool.astro.deployments.prod]\n" +
			"deployment = 'clx123'\nworkspace = 'ws-default'\ntarget = 'astro'\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir, path := linkProject(t, inheritingFixture+links)
			saveLink(t, dir, Link{Name: "prod", Kind: manifest.KindAstro, Workspace: "ws-default", Deployment: "clx456"})
			assert.Equal(t, map[string]any{"workspace": "ws-default", "target": "astro"}, ownKeys(t, path, "prod"))

			moved := strings.Replace(readFile(t, path), "workspace = 'ws-default'\ndomain", "workspace = 'ws-moved'\ndomain", 1)
			require.NoError(t, os.WriteFile(path, []byte(moved), 0o600))
			_, got := onlyLink(t, path)
			assert.Equal(t, "ws-default", got.Workspace, "the pin was dropped")
		})
	}
}

// A pinned key takes the new value, and stays the link's own though the new
// value is the default.
func TestSaveLinkAPinnedKeyTakesTheNewValue(t *testing.T) {
	dir, path := linkProject(t, inheritingFixture+"\n[tool.astro.deployments]\nprod = {deployment = 'clx123', workspace = 'ws-old'}\n")
	saveLink(t, dir, Link{Name: "prod", Kind: manifest.KindAstro, Workspace: "ws-default", Deployment: "clx123"})
	assert.Equal(t, "ws-default", ownKeys(t, path, "prod")["workspace"])
}

// composerFixture sets a project workspace, so astro links in these tests load.
const composerFixture = "[project]\nname = 'demo'\nrequires-python = '>=3.10'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\nworkspace = 'ws_abc123'\n"

// targetField is one field of [tool.astro.targets.<kind>] as the parser reads
// it, which is what the CLI's address lookups read too.
func targetField(t *testing.T, path string, kind manifest.LinkKind, field string) any {
	t.Helper()
	return loadLinks(t, path).Astro.Targets[string(kind)][field]
}

// A Composer link writes the coordinates its address lookup reads.
func TestSaveLinkWritesTheComposerCoordinates(t *testing.T) {
	dir, path := linkProject(t, composerFixture)
	saveLink(t, dir, Link{
		Name: "gcp-prod", Kind: manifest.KindComposer, Environment: "orders-prod",
		TargetProject: " acme-data ", TargetLocation: "us-central1",
	})
	assert.Equal(t, "acme-data", targetField(t, path, manifest.KindComposer, "project"))
	assert.Equal(t, "us-central1", targetField(t, path, manifest.KindComposer, "location"))
}

// The section is shared, so saving one Composer link's coordinates moves every
// Composer link.
func TestSaveLinkComposerCoordinatesAreShared(t *testing.T) {
	dir, path := linkProject(t, composerFixture)
	saveLink(t, dir, composerLink("gcp-a"))
	b := composerLink("gcp-b")
	b.TargetLocation = "europe-west1"
	saveLink(t, dir, b)
	m := loadLinks(t, path)
	assert.Len(t, m.Astro.Deployments, 2)
	assert.Equal(t, "europe-west1", m.Astro.Targets["composer"]["location"])
}

// Writing the coordinates leaves the rest of the section as the person wrote
// it: comments, the header, key order, and any key the writer does not know.
func TestSaveLinkLeavesTheComposerSectionIntact(t *testing.T) {
	dir, path := linkProject(t, composerFixture+
		"\n# Where our Composer environments live.\n"+
		"[tool.astro.targets.composer]\n"+
		"# The data-platform project, not the app one.\n"+
		"project = 'old-project'\n"+
		"location = 'us-east1'\n"+
		"image-repository = 'gcr.io/acme/airflow'\n")
	saveLink(t, dir, composerLink("gcp-prod"))
	out := readFile(t, path)
	for _, want := range []string{
		"# Where our Composer environments live.",
		"[tool.astro.targets.composer]",
		"# The data-platform project, not the app one.",
		"image-repository = 'gcr.io/acme/airflow'",
		"project = 'acme-data'",
	} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "old-project")
	assert.Less(t, strings.Index(out, "project = "), strings.Index(out, "location = "), "key order was rewritten:\n%s", out)
}

// A Composer link needs both coordinates, and one of spaces counts as none.
func TestSaveLinkAComposerLinkNeedsBothCoordinates(t *testing.T) {
	for _, tc := range []struct {
		name              string
		project, location string
		want              error
	}{
		{"no project", "", "us-central1", ErrComposerProjectRequired},
		{"spaces for a project", "   ", "us-central1", ErrComposerProjectRequired},
		{"no location", "acme-data", "", ErrComposerLocationRequired},
		{"spaces for a location", "acme-data", "\t", ErrComposerLocationRequired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, path := linkProject(t, composerFixture)
			l := Link{Name: "gcp", Kind: manifest.KindComposer, Environment: "e", TargetProject: tc.project, TargetLocation: tc.location}
			err := SaveLink(dir, nil, l)
			require.ErrorIs(t, err, tc.want)
			require.ErrorIs(t, err, ErrInvalidLink)
			assert.Equal(t, composerFixture, readFile(t, path))
		})
	}
}

// Another kind's save leaves the Composer section alone.
func TestSaveLinkANonComposerSaveLeavesTheComposerSectionAlone(t *testing.T) {
	const section = "\n[tool.astro.targets.composer]\nproject = 'acme-data'\nlocation = 'us-central1'\n"
	dir, path := linkProject(t, composerFixture+section)
	saveLink(t, dir, Link{Name: "aws-prod", Kind: manifest.KindMWAA, Environment: "my-mwaa", TargetProject: "other", TargetLocation: "elsewhere"})
	assert.Contains(t, readFile(t, path), section)
}

// An MWAA region is written where the CLI's MWAA transport reads it, trimmed,
// and a second MWAA link with no region leaves it.
func TestSaveLinkWritesTheMWAARegion(t *testing.T) {
	dir, path := linkProject(t, composerFixture)
	saveLink(t, dir, Link{Name: "aws-prod", Kind: manifest.KindMWAA, Environment: "orders-prod", TargetRegion: " eu-west-1 "})
	saveLink(t, dir, Link{Name: "aws-dev", Kind: manifest.KindMWAA, Environment: "orders-dev"})
	assert.Equal(t, "eu-west-1", targetField(t, path, manifest.KindMWAA, "region"))
}

// A save with no region leaves the shared one alone, comment and all.
func TestSaveLinkWithNoRegionLeavesTheMWAASectionAlone(t *testing.T) {
	const section = "\n[tool.astro.targets.mwaa]\n# Our AWS account's home region.\nregion = 'us-east-2'\n"
	dir, path := linkProject(t, composerFixture+section)
	saveLink(t, dir, Link{Name: "aws", Kind: manifest.KindMWAA, Environment: "orders", TargetRegion: "  "})
	assert.Contains(t, readFile(t, path), section)
}

// Clearing removes only the region leaf: the section's header, the comment
// above it and its other keys stay.
func TestSaveLinkClearingTheRegionRemovesOnlyThatKey(t *testing.T) {
	dir, path := linkProject(t, composerFixture+
		"\n# Our AWS account.\n"+
		"[tool.astro.targets.mwaa]\n"+
		"region = 'us-east-2'\n"+
		"bucket = 'acme-dags'\n")
	saveLink(t, dir, Link{Name: "aws", Kind: manifest.KindMWAA, Environment: "orders", TargetRegion: " ", ClearTargetRegion: true})
	out := readFile(t, path)
	assert.NotContains(t, out, "region")
	for _, want := range []string{"[tool.astro.targets.mwaa]", "# Our AWS account.", "bucket = 'acme-dags'"} {
		assert.Contains(t, out, want)
	}
	assert.Nil(t, targetField(t, path, manifest.KindMWAA, "region"))
}

// A clear alongside a value writes the value.
func TestSaveLinkAClearWithARegionWritesIt(t *testing.T) {
	dir, path := linkProject(t, composerFixture+"\n[tool.astro.targets.mwaa]\nregion = 'us-east-2'\n")
	saveLink(t, dir, Link{Name: "aws", Kind: manifest.KindMWAA, Environment: "orders", TargetRegion: "eu-west-1", ClearTargetRegion: true})
	assert.Equal(t, "eu-west-1", targetField(t, path, manifest.KindMWAA, "region"))
}

// Writing the region replaces only that key.
func TestSaveLinkWritingTheRegionLeavesTheMWAASectionIntact(t *testing.T) {
	dir, path := linkProject(t, composerFixture+
		"\n[tool.astro.targets.mwaa]\n"+
		"# Our AWS account's home region.\n"+
		"region = 'us-east-2'\n"+
		"bucket = 'acme-dags'\n")
	saveLink(t, dir, Link{Name: "aws", Kind: manifest.KindMWAA, Environment: "orders", TargetRegion: "eu-west-1"})
	out := readFile(t, path)
	for _, want := range []string{"[tool.astro.targets.mwaa]", "# Our AWS account's home region.", "region = 'eu-west-1'", "bucket = 'acme-dags'"} {
		assert.Contains(t, out, want)
	}
}

// Only an MWAA link writes a region, and only an MWAA link clears one.
func TestSaveLinkOnlyAnMWAALinkTouchesTheRegion(t *testing.T) {
	dir, path := linkProject(t, composerFixture)
	saveLink(t, dir, Link{Name: "prod", Kind: manifest.KindAstro, Deployment: "clx1", TargetRegion: "eu-west-1"})
	assert.NotContains(t, readFile(t, path), "region")

	const section = "\n[tool.astro.targets.mwaa]\nregion = 'us-east-2'\n"
	dir, path = linkProject(t, composerFixture+section)
	saveLink(t, dir, Link{Name: "prod", Kind: manifest.KindAstro, Deployment: "clx1", ClearTargetRegion: true})
	assert.Contains(t, readFile(t, path), section)
}

// RemoveLink removes a link in either spelling, reports whether it did, and is
// idempotent. The rest of the file stays.
func TestRemoveLink(t *testing.T) {
	for name, body := range map[string]string{
		"header": linkFixture + "workspace = 'W'\n\n[tool.astro.deployments.prod]\ndeployment = 'clx1'\n\n[tool.astro.deployments.dev]\ndeployment = 'clx2'\n",
		"inline": linkFixture + "workspace = 'W'\n\n[tool.astro.deployments]\nprod = { deployment = 'clx1' }\ndev = { deployment = 'clx2' }\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir, path := linkProject(t, body)
			removed, err := RemoveLink(dir, nil, "prod")
			require.NoError(t, err)
			assert.True(t, removed)
			m := loadLinks(t, path)
			assert.NotContains(t, m.Astro.Deployments, "prod")
			assert.Contains(t, m.Astro.Deployments, "dev")
			assert.Contains(t, readFile(t, path), userComment)

			after := readFile(t, path)
			removed, err = RemoveLink(dir, nil, "prod")
			require.NoError(t, err)
			assert.False(t, removed)
			assert.Equal(t, after, readFile(t, path))
		})
	}
}

// RemoveLink never creates a manifest.
func TestRemoveLinkRefusesAMissingManifest(t *testing.T) {
	_, err := RemoveLink(t.TempDir(), nil, "prod")
	require.ErrorIs(t, err, manifest.ErrNotFound)
}

// Marking a default clears the previous holder, since the manifest allows one.
func TestSetDefaultLinkLeavesOneDefault(t *testing.T) {
	for name, body := range map[string]string{
		"header": linkFixture + "workspace = 'W'\n\n[tool.astro.deployments.a]\ndeployment = 'a'\ndefault = true\n\n[tool.astro.deployments.b]\ndeployment = 'b'\n",
		"inline": linkFixture + "workspace = 'W'\n\n[tool.astro.deployments]\na = { deployment = 'a', default = true }\nb = { deployment = 'b' }\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir, path := linkProject(t, body)
			require.NoError(t, SetDefaultLink(dir, nil, "b"))
			m := loadLinks(t, path)
			assert.False(t, m.Astro.Deployments["a"].Default)
			assert.True(t, m.Astro.Deployments["b"].Default)
			assert.Contains(t, readFile(t, path), userComment)

			// Again is a no-op.
			after := readFile(t, path)
			require.NoError(t, SetDefaultLink(dir, nil, "b"))
			assert.Equal(t, after, readFile(t, path))

			// An empty name clears it.
			require.NoError(t, SetDefaultLink(dir, nil, ""))
			links := loadLinks(t, path).Astro.Deployments
			for n := range links {
				assert.False(t, links[n].Default, "%s is still the default", n)
			}
		})
	}
}

// A name the manifest has no link for is refused, and nothing changes: the
// previous default is not cleared on the way to failing.
func TestSetDefaultLinkRefusesAnUnknownName(t *testing.T) {
	body := linkFixture + "workspace = 'W'\n\n[tool.astro.deployments.a]\ndeployment = 'a'\ndefault = true\n"
	dir, path := linkProject(t, body)
	err := SetDefaultLink(dir, nil, "nope")
	require.ErrorIs(t, err, ErrNoSuchLink)
	assert.Equal(t, body, readFile(t, path))
}

// Linking writes [tool.astro] workspace and domain, the keys the CLI reads, and
// leaves the rest of the file alone. Relinking replaces both.
func TestSetWorkspaceLinkWritesWorkspaceAndDomain(t *testing.T) {
	dir, path := linkProject(t, linkFixture)
	require.NoError(t, setWorkspace(t, dir, "cmws123", "astronomer.io"))
	m := loadLinks(t, path)
	assert.Equal(t, "cmws123", m.Astro.Workspace)
	assert.Equal(t, "astronomer.io", m.Astro.Domain)
	assert.Contains(t, readFile(t, path), userComment)

	require.NoError(t, setWorkspace(t, dir, "cmws456", "astronomer-dev.io"))
	m = loadLinks(t, path)
	assert.Equal(t, "cmws456", m.Astro.Workspace)
	assert.Equal(t, "astronomer-dev.io", m.Astro.Domain)
	assert.Equal(t, 1, strings.Count(readFile(t, path), "domain = "))
}

// A workspace cannot be linked without its host, and nothing is written.
func TestSetWorkspaceLinkNeedsADomain(t *testing.T) {
	dir, path := linkProject(t, linkFixture)
	for _, domain := range []string{"", "  ", "https://"} {
		require.ErrorIs(t, setWorkspace(t, dir, "cmws789", domain), ErrWorkspaceDomainRequired)
	}
	assert.Equal(t, linkFixture, readFile(t, path))
}

// The domain is stored the way `astro login` stores a login's.
func TestSetWorkspaceLinkNormalizesTheDomain(t *testing.T) {
	dir, path := linkProject(t, linkFixture)
	require.NoError(t, setWorkspace(t, dir, " cmws123 ", "https://cloud.Astronomer-Dev.io/"))
	m := loadLinks(t, path)
	assert.Equal(t, "cmws123", m.Astro.Workspace)
	assert.Equal(t, "astronomer-dev.io", m.Astro.Domain)
	assert.NotContains(t, readFile(t, path), "https://")
}

// linkWorkspaces is each link's resolved workspace.
func linkWorkspaces(t *testing.T, path string) map[string]string {
	t.Helper()
	out := map[string]string{}
	links := loadLinks(t, path).Astro.Deployments
	for name := range links {
		out[name] = links[name].Workspace
	}
	return out
}

// Every workspace change, a switch or an unlink, first writes the old workspace
// onto each astro link inheriting it, so none moves. A link naming its own
// workspace, and a link of another kind, are left alone.
func TestSetWorkspaceLinkPinsInheritingLinks(t *testing.T) {
	const body = linkFixture + `workspace = 'ws_A'
domain = 'astronomer.io'

[tool.astro.deployments]
stage = { deployment = 'dep-stage' }

[tool.astro.deployments.dev]
deployment = 'dep-dev'

[tool.astro.deployments.prod]
workspace = 'ws_C'
deployment = 'dep-prod'

[tool.astro.deployments.orders]
target = 'mwaa'
environment = 'orders-prod'
`
	want := map[string]string{"stage": "ws_A", "dev": "ws_A", "prod": "ws_C", "orders": ""}

	t.Run("switch", func(t *testing.T) {
		dir, path := linkProject(t, body)
		pinned, err := SetWorkspaceLink(dir, nil, "ws_B", "astronomer.io", "")
		require.NoError(t, err)
		assert.Equal(t, []string{"dev", "stage"}, pinned)
		assert.Equal(t, "ws_B", loadLinks(t, path).Astro.Workspace)
		assert.Equal(t, want, linkWorkspaces(t, path))
		assert.Contains(t, readFile(t, path), userComment)
		assert.NotContains(t, ownKeys(t, path, "orders"), "workspace")
	})
	t.Run("unlink", func(t *testing.T) {
		dir, path := linkProject(t, body)
		pinned, err := SetWorkspaceLink(dir, nil, "", "", "")
		require.NoError(t, err)
		assert.Equal(t, []string{"dev", "stage"}, pinned)
		m := loadLinks(t, path)
		assert.Empty(t, m.Astro.Workspace)
		assert.Empty(t, m.Astro.Domain)
		assert.NotContains(t, readFile(t, path), "domain = ")
		assert.Equal(t, want, linkWorkspaces(t, path))
	})
}

// Linking the workspace already linked copies nothing and writes nothing.
func TestSetWorkspaceLinkTheSameWorkspaceIsANoOp(t *testing.T) {
	const body = linkFixture + "workspace = 'ws_A'\ndomain = 'astronomer.io'\n\n[tool.astro.deployments.dev]\ndeployment = 'dep-dev'\n"
	dir, path := linkProject(t, body)
	pinned, err := SetWorkspaceLink(dir, nil, "ws_A", "astronomer.io", "")
	require.NoError(t, err)
	assert.Empty(t, pinned)
	assert.Equal(t, body, readFile(t, path))

	// The same workspace on another host moves the domain alone.
	require.NoError(t, setWorkspace(t, dir, "ws_A", "astronomer-dev.io"))
	assert.Empty(t, ownKeys(t, path, "dev"), "a domain change pinned the link")
	assert.Equal(t, "astronomer-dev.io", loadLinks(t, path).Astro.Domain)
}

// Linking writes the organization beside the workspace and domain, trimmed, and
// relinking the same three writes nothing.
func TestSetWorkspaceLinkWritesTheOrganization(t *testing.T) {
	dir, path := linkProject(t, linkFixture)
	_, err := SetWorkspaceLink(dir, nil, "cmws123", "astronomer.io", " clorg ")
	require.NoError(t, err)
	m := loadLinks(t, path)
	assert.Equal(t, "cmws123", m.Astro.Workspace)
	assert.Equal(t, "astronomer.io", m.Astro.Domain)
	assert.Equal(t, "clorg", m.Astro.Organization)
	assert.Contains(t, readFile(t, path), "organization = ")

	before := readFile(t, path)
	_, err = SetWorkspaceLink(dir, nil, "cmws123", "astronomer.io", "clorg")
	require.NoError(t, err)
	assert.Equal(t, before, readFile(t, path))

	// The same workspace in another organization moves the organization alone.
	_, err = SetWorkspaceLink(dir, nil, "cmws123", "astronomer.io", "clother")
	require.NoError(t, err)
	assert.Equal(t, "clother", loadLinks(t, path).Astro.Organization)
	assert.Equal(t, 1, strings.Count(readFile(t, path), "organization = "))
}

// An empty organization for the workspace already linked is "not given": the
// organization stays, byte for byte, and so it does when only the domain moves.
func TestSetWorkspaceLinkWithoutAnOrganizationKeepsTheLinkedWorkspaces(t *testing.T) {
	body := linkFixture + "workspace = 'ws_A'\ndomain = 'astronomer.io'\norganization = 'clorg'\n"
	dir, path := linkProject(t, body)
	_, err := SetWorkspaceLink(dir, nil, "ws_A", "astronomer.io", "")
	require.NoError(t, err)
	assert.Equal(t, body, readFile(t, path))

	_, err = SetWorkspaceLink(dir, nil, "ws_A", "astronomer-dev.io", "")
	require.NoError(t, err)
	m := loadLinks(t, path)
	assert.Equal(t, "clorg", m.Astro.Organization)
	assert.Equal(t, "astronomer-dev.io", m.Astro.Domain)
}

// An empty organization with another workspace removes the old one, which was
// the old workspace's, so the new one reads under the login's own organization.
func TestSetWorkspaceLinkWithoutAnOrganizationRemovesIt(t *testing.T) {
	dir, path := linkProject(t, linkFixture+"workspace = 'ws_A'\ndomain = 'astronomer.io'\norganization = 'clorg'\n")
	_, err := SetWorkspaceLink(dir, nil, "ws_B", "astronomer.io", "")
	require.NoError(t, err)
	m := loadLinks(t, path)
	assert.Equal(t, "ws_B", m.Astro.Workspace)
	assert.Empty(t, m.Astro.Organization)
	assert.NotContains(t, readFile(t, path), "organization = ")
}

// Unlinking removes the organization with the workspace and domain, since the
// parser refuses an organization with no workspace, and still pins the links
// that inherited the workspace.
func TestSetWorkspaceLinkUnlinkingClearsAllThree(t *testing.T) {
	dir, path := linkProject(t, linkFixture+"workspace = 'ws_A'\ndomain = 'astronomer.io'\norganization = 'clorg'\n\n[tool.astro.deployments.dev]\ndeployment = 'dep-dev'\n")
	pinned, err := SetWorkspaceLink(dir, nil, "", "", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"dev"}, pinned)
	m := loadLinks(t, path)
	assert.Empty(t, m.Astro.Workspace)
	assert.Empty(t, m.Astro.Domain)
	assert.Empty(t, m.Astro.Organization)
	body := readFile(t, path)
	for _, key := range []string{"domain = ", "organization = "} {
		assert.NotContains(t, body, key)
	}
	assert.Equal(t, map[string]string{"dev": "ws_A"}, linkWorkspaces(t, path))
}

// Unlinking a project that links nothing writes nothing.
func TestSetWorkspaceLinkUnlinkingAnUnlinkedProjectIsANoOp(t *testing.T) {
	dir, path := linkProject(t, linkFixture)
	require.NoError(t, setWorkspace(t, dir, "", ""))
	assert.Equal(t, linkFixture, readFile(t, path))
}

// A project with no manifest has nowhere to link, and none is created.
func TestSetWorkspaceLinkNeverCreatesAManifest(t *testing.T) {
	dir := t.TempDir()
	err := setWorkspace(t, dir, "cmws123", "astronomer.io")
	require.True(t, errors.Is(err, manifest.ErrNotFound), "err = %v", err)
	_, statErr := os.Stat(filepath.Join(dir, manifest.Marker))
	assert.True(t, os.IsNotExist(statErr))
}
