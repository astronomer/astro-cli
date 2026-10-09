package local

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/internal/vaultenv"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/secrets"
	"github.com/astronomer/astro-cli/pkg/secrets/secretstest"
)

const linkTestURI = "postgres://u:p@h/db"

// run executes one command from dir and returns stdout and stderr.
func run(t *testing.T, dir string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	d, out, errOut := envDeps(t, dir, "")
	err = execute(t, d, append([]string{"local", "env"}, args...)...)
	return out.String(), errOut.String(), err
}

func mustRun(t *testing.T, dir string, args ...string) (stdout, stderr string) {
	t.Helper()
	out, errOut, err := run(t, dir, args...)
	if err != nil {
		t.Fatalf("%v: %v\nstderr: %s", args, err, errOut)
	}
	return out, errOut
}

// setGlobalConn stores a global connection in the vault reaching every
// project, as a global made before link state existed does.
func setGlobalConn(t *testing.T, dir, name string) {
	t.Helper()
	mustRun(t, dir, "connection", "set", name, "--value", linkTestURI, "--global", "--auto-link")
}

func canonPath(t *testing.T, dir string) string {
	t.Helper()
	p, err := localrt.CanonicalPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func vaultDir(t *testing.T) string {
	t.Helper()
	dir, err := secrets.DefaultDir()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// reachOf reads the index the way Astro Desktop does.
func reachOf(t *testing.T, vaultKey string) secrets.Reach {
	t.Helper()
	links, err := secrets.OpenLinks(vaultDir(t))
	if err != nil {
		t.Fatal(err)
	}
	return links.ReachOf(vaultKey)
}

func linkGetJSON(t *testing.T, dir string, args ...string) envValue {
	t.Helper()
	out, _ := mustRun(t, dir, append(args, "--output", "json")...)
	var v envValue
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	return v
}

// Linking narrows an entry that reached every project, says so, and get
// reports the new reach in both forms, keeping the text form's stdout the
// value alone.
func TestEnvLinkThenGetShowsTheReach(t *testing.T) {
	dir := envProject(t, "")
	setGlobalConn(t, dir, "warehouse")

	v := linkGetJSON(t, dir, "connection", "get", "warehouse")
	if v.Reach == nil || !v.Reach.AutoLink {
		t.Fatalf("before any link, reach = %+v, want everywhere", v.Reach)
	}

	out, _ := mustRun(t, dir, "connection", "link", "warehouse")
	home := canonPath(t, dir)
	if !strings.Contains(out, "now reaches only "+home) {
		t.Errorf("narrowing an everywhere entry should say so, got:\n%s", out)
	}

	v = linkGetJSON(t, dir, "connection", "get", "warehouse")
	want := reachJSON{Projects: []reachPath{{Path: home, Exists: true}}}
	if v.Reach == nil || v.Reach.AutoLink || !slices.Equal(v.Reach.Projects, want.Projects) {
		t.Errorf("reach = %+v, want %+v", v.Reach, want)
	}

	stdout, stderr := mustRun(t, dir, "connection", "get", "warehouse")
	if !strings.Contains(stderr, "Reach: "+home) {
		t.Errorf("text get should show the reach, stderr:\n%s", stderr)
	}
	if strings.Contains(stdout, "Reach") {
		t.Errorf("stdout must stay the value alone:\n%s", stdout)
	}

	// A second project: listed alongside, and a path that goes away is marked.
	other := t.TempDir()
	mustRun(t, dir, "connection", "link", "warehouse", other)
	if err := os.RemoveAll(other); err != nil {
		t.Fatal(err)
	}
	_, stderr = mustRun(t, dir, "connection", "get", "warehouse", "--global")
	if !strings.Contains(stderr, "(missing)") || !strings.Contains(stderr, home) {
		t.Errorf("a gone path should be marked missing, stderr:\n%s", stderr)
	}

	// A project secret has no reach.
	mustRun(t, dir, "connection", "set", "local_db", "--value", linkTestURI, "--project")
	if v := linkGetJSON(t, dir, "connection", "get", "local_db"); v.Reach != nil {
		t.Errorf("a project secret reported a reach: %+v", v.Reach)
	}
}

// From a linked worktree, link names the project home so the main checkout and
// every other worktree share it; --this-checkout names the worktree alone.
func TestEnvLinkFromAWorktreeLinksTheHome(t *testing.T) {
	envProject(t, "")
	l := secretstest.NewLayout(t)
	wt := l.Dir(secretstest.PlaceCustomWorktree)
	setGlobalConn(t, wt, "warehouse")
	mustRun(t, wt, "variable", "set", "SLACK_TOKEN", "--value", "x", "--global")

	mustRun(t, wt, "connection", "link", "warehouse")
	if got, want := reachOf(t, "conn:global:warehouse").Projects, []string{l.Checkout(secretstest.PlaceMain).Path}; !slices.Equal(got, want) {
		t.Errorf("link from a worktree = %v, want the main checkout %v", got, want)
	}

	mustRun(t, wt, "variable", "link", "SLACK_TOKEN", "--this-checkout")
	if got, want := reachOf(t, "env:global:SLACK_TOKEN").Projects, []string{l.Checkout(secretstest.PlaceCustomWorktree).Path}; !slices.Equal(got, want) {
		t.Errorf("--this-checkout = %v, want the worktree %v", got, want)
	}

	// Unlinking from the worktree stops it being reached, whichever way it
	// was linked.
	mustRun(t, wt, "connection", "unlink", "warehouse")
	if r := reachOf(t, "conn:global:warehouse"); r.Includes(l.Checkout(secretstest.PlaceCustomWorktree)) {
		t.Errorf("after unlink the worktree is still reached: %+v", r)
	}
}

// What the CLI writes, every checkout reads the same way through the CLI's
// resolver and through the predicate Astro Desktop applies.
func TestEnvLinkAgreesWithTheSharedPredicate(t *testing.T) {
	envProject(t, "")
	l := secretstest.NewLayout(t)
	main := l.Dir(secretstest.PlaceMain)
	setGlobalConn(t, main, "warehouse")
	mustRun(t, main, "connection", "link", "warehouse")
	r := reachOf(t, "conn:global:warehouse")

	reached := map[secretstest.Place]bool{
		secretstest.PlaceMain: true, secretstest.PlaceWorktree: true, secretstest.PlaceCustomWorktree: true,
		secretstest.PlaceSymlink: true, secretstest.PlaceCaseVariant: true,
	}
	for _, p := range []secretstest.Place{
		secretstest.PlaceMain, secretstest.PlaceMainSub, secretstest.PlaceWorktree, secretstest.PlaceCustomWorktree,
		secretstest.PlaceCustomWorktreeSub, secretstest.PlaceSymlink, secretstest.PlaceCaseVariant,
		secretstest.PlaceOther, secretstest.PlaceSandbox, secretstest.PlaceOutside,
	} {
		dir := l.Dir(p)
		if dir == "" {
			continue // this filesystem cannot express the place
		}
		desktop := r.Includes(l.Checkout(p))
		cli := false
		for _, tier := range vaultenv.Load(dir).Tiers() {
			for _, e := range tier.Entries {
				if e.Name == "warehouse" && tier.Scope == localenv.ScopeGlobal && !e.Unlinked {
					cli = true
				}
			}
		}
		if desktop != reached[p] || cli != reached[p] {
			t.Errorf("%s: desktop predicate %v, CLI resolver %v, want %v", p, desktop, cli, reached[p])
		}
	}
}

func TestEnvUnlinkToEmptyWarns(t *testing.T) {
	dir := envProject(t, "")
	setGlobalConn(t, dir, "warehouse")
	mustRun(t, dir, "connection", "link", "warehouse")

	out, stderr := mustRun(t, dir, "connection", "unlink", "warehouse")
	if !strings.Contains(stderr, "reaches no project") {
		t.Errorf("unlinking the last project should warn, stderr:\n%s", stderr)
	}
	if !strings.Contains(out, "Reach: not linked (no project)") {
		t.Errorf("out:\n%s", out)
	}
	if r := reachOf(t, "conn:global:warehouse"); r.Everywhere || len(r.Projects) != 0 {
		t.Errorf("reach = %+v, want a row reaching no project", r)
	}
}

func TestEnvUnlinkOnAnAutoLinkedEntryErrors(t *testing.T) {
	dir := envProject(t, "")
	setGlobalConn(t, dir, "warehouse")
	_, _, err := run(t, dir, "connection", "unlink", "warehouse")
	if err == nil || !strings.Contains(err.Error(), "is auto-linked to every project") || !strings.Contains(err.Error(), "connection link warehouse") {
		t.Fatalf("err = %v, want one naming the link command", err)
	}
	if _, statErr := os.Stat(secrets.LinksPath(vaultDir(t))); !os.IsNotExist(statErr) {
		t.Errorf("a refused unlink wrote the index: %v", statErr)
	}
}

func TestEnvLinkAutoLinkRemovesTheRow(t *testing.T) {
	dir := envProject(t, "")
	setGlobalConn(t, dir, "warehouse")
	mustRun(t, dir, "connection", "link", "warehouse")
	out, _ := mustRun(t, dir, "connection", "link", "warehouse", "--auto-link")
	if !strings.Contains(out, "is now auto-linked to every project") {
		t.Errorf("out:\n%s", out)
	}
	raw, err := os.ReadFile(secrets.LinksPath(vaultDir(t)))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "warehouse") {
		t.Errorf("the row survived --auto-link:\n%s", raw)
	}
	if _, _, err := run(t, dir, "connection", "link", "warehouse", "--auto-link", dir); err == nil {
		t.Error("--auto-link with a directory should be refused")
	}
}

// Only a global vault entry has link state; everything else is refused with
// the reason, and nothing is written.
func TestEnvLinkRefusesAnythingButAGlobalVaultEntry(t *testing.T) {
	dir := envProject(t, "")
	mustRun(t, dir, "connection", "set", "local_db", "--value", linkTestURI, "--project")
	writeLegacyGlobal(t, "PLAIN=x\n")
	setGlobalConn(t, dir, "warehouse")

	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"project secret": {[]string{"connection", "link", "local_db"}, "already reaches only this project"},
		"legacy file":    {[]string{"variable", "link", "PLAIN"}, "holds no global variable PLAIN"},
		"missing":        {[]string{"connection", "link", "nope"}, "holds no global connection nope"},
		"--project":      {[]string{"connection", "link", "warehouse", "--project"}, "global vault entries only"},
		"unlink missing": {[]string{"airflow-variable", "unlink", "nope"}, "holds no global airflow-variable nope"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := run(t, dir, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
	if _, err := os.Stat(secrets.LinksPath(vaultDir(t))); !os.IsNotExist(err) {
		t.Errorf("a refused link wrote the index: %v", err)
	}
}

// An undeclared global linked elsewhere is not part of this project, so list
// leaves it out; --all shows it, marked not linked here with the command that
// links it. A declared name it would have supplied lists as absent, exactly
// as if no global existed, and --all marks that row too. get explains the
// absence the same way the start gate does.
func TestEnvListMarksNotLinkedHere(t *testing.T) {
	dir := envProject(t, "[tool.astro.env.connections]\ndeclared_db = {}\n")
	other := t.TempDir()
	for _, name := range []string{"warehouse", "declared_db"} {
		setGlobalConn(t, dir, name)
		mustRun(t, dir, "connection", "link", name, other)
	}

	out, _ := mustRun(t, dir, "connection", "list")
	if strings.Contains(out, "warehouse") {
		t.Errorf("list showed a global that does not reach this project:\n%s", out)
	}
	if strings.Contains(out, "not linked here") || !strings.Contains(out, "declared_db") {
		t.Errorf("list should show declared_db absent and unmarked:\n%s", out)
	}
	out, _ = mustRun(t, dir, "connection", "list", "--output", "json")
	var plain localenv.ListItem
	for _, it := range decodeEnvList(t, out) {
		if it.Name == "declared_db" {
			plain = it
		}
	}
	if plain.Source != localenv.SourceAbsent || plain.NotLinkedHere || plain.LinkHint != "" {
		t.Errorf("declared_db without --all = %+v, want a plain absent row", plain)
	}

	out, _ = mustRun(t, dir, "list", "--all", "--output", "json")
	rows := map[string]localenv.ListItem{}
	for _, it := range decodeEnvList(t, out) {
		rows[it.Name] = it
	}
	for _, name := range []string{"warehouse", "declared_db"} {
		it := rows[name]
		if !it.NotLinkedHere || it.LinkHint != "astro local env connection link "+name {
			t.Errorf("%s row = %+v, want not linked here with a link hint", name, it)
		}
		// Its way out is the link, not a value or an undeclare.
		if it.SetHint != "" || it.UndeclareHint != "" {
			t.Errorf("%s row = %+v, want no set or undeclare hint", name, it)
		}
	}

	out, _ = mustRun(t, dir, "connection", "list", "--all")
	if !strings.Contains(out, "not linked here (link: astro local env connection link warehouse)") {
		t.Errorf("text list:\n%s", out)
	}

	_, _, err := run(t, dir, "connection", "get", "warehouse")
	if err == nil || !strings.Contains(err.Error(), "connection link warehouse") {
		t.Errorf("get of a global linked elsewhere = %v, want the link command", err)
	}
}

// Deleting a global's value deletes its row, so a re-created entry of the same
// name starts from every project rather than a pin nobody can see.
func TestEnvDeleteRemovesTheLinkRow(t *testing.T) {
	dir := envProject(t, "")
	setGlobalConn(t, dir, "warehouse")
	setGlobalConn(t, dir, "kept")
	mustRun(t, dir, "connection", "link", "warehouse")
	mustRun(t, dir, "connection", "link", "kept")

	mustRun(t, dir, "connection", "delete", "warehouse", "--global")
	if r := reachOf(t, "conn:global:warehouse"); !r.Everywhere {
		t.Errorf("the row survived the delete: %+v", r)
	}
	if r := reachOf(t, "conn:global:kept"); r.Everywhere {
		t.Error("another entry's row went with it")
	}

	// A vault delete works the same way.
	mustRun(t, dir, "connection", "delete", "kept", "--global")
	if r := reachOf(t, "conn:global:kept"); !r.Everywhere {
		t.Errorf("the row survived delete: %+v", r)
	}
}

// A delete whose row cannot be removed still deletes the value, and says what
// was left behind.
func TestEnvDeleteWithAnUnusableIndexKeepsTheRowAndWarns(t *testing.T) {
	dir := envProject(t, "")
	setGlobalConn(t, dir, "warehouse")
	writeIndex(t, `{"version":99,"links":{}}`)
	_, stderr := mustRun(t, dir, "connection", "delete", "warehouse", "--global")
	if !strings.Contains(stderr, "link row") {
		t.Errorf("stderr:\n%s", stderr)
	}
	if _, _, err := run(t, dir, "connection", "get", "warehouse", "--global"); err == nil {
		t.Error("the value survived")
	}
}

func writeIndex(t *testing.T, body string) {
	t.Helper()
	if err := os.MkdirAll(vaultDir(t), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vaultDir(t), "links.idx"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A corrupt or newer index is never rewritten, and get and list show that no
// global resolves while it stands.
func TestEnvLinkRefusesAnUnusableIndex(t *testing.T) {
	for name, body := range map[string]string{"corrupt": "{not json", "newer": `{"version":99,"links":{}}`} {
		t.Run(name, func(t *testing.T) {
			dir := envProject(t, "")
			setGlobalConn(t, dir, "warehouse")
			writeIndex(t, body)

			for _, args := range [][]string{
				{"connection", "link", "warehouse"},
				{"connection", "link", "warehouse", "--auto-link"},
				{"connection", "unlink", "warehouse"},
			} {
				_, _, err := run(t, dir, args...)
				if err == nil || !strings.Contains(err.Error(), "links.idx") || !strings.Contains(err.Error(), "Nothing was changed") {
					t.Errorf("%v: err = %v, want a refusal naming the index", args, err)
				}
			}
			raw, err := os.ReadFile(filepath.Join(vaultDir(t), "links.idx"))
			if err != nil || string(raw) != body {
				t.Errorf("the index was rewritten: %q, %v", raw, err)
			}

			v := linkGetJSON(t, dir, "connection", "get", "warehouse", "--global")
			if v.Reach == nil || v.Reach.Error == "" || v.Reach.AutoLink {
				t.Errorf("reach = %+v, want the index error", v.Reach)
			}
			out, _ := mustRun(t, dir, "connection", "list", "--all")
			if !strings.Contains(out, "not linked here: link index") {
				t.Errorf("list:\n%s", out)
			}
		})
	}
}

// Link and unlink sit after the declaration verbs on every noun, and their
// directories are positional, so the persistent --project bool never takes
// one as its value.
func TestEnvLinkTakesDirectoriesPositionally(t *testing.T) {
	dir := envProject(t, "")
	setGlobalConn(t, dir, "warehouse")
	a, b := t.TempDir(), t.TempDir()
	var out bytes.Buffer
	o, _ := mustRun(t, dir, "connection", "link", "warehouse", a, b)
	out.WriteString(o)
	got := reachOf(t, "conn:global:warehouse").Projects
	want := []string{canonPath(t, a), canonPath(t, b)}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("projects = %v, want %v (out %s)", got, want, out.String())
	}
	mustRun(t, dir, "connection", "unlink", "warehouse", a)
	if got := reachOf(t, "conn:global:warehouse").Projects; !slices.Equal(got, []string{canonPath(t, b)}) {
		t.Errorf("after unlink = %v", got)
	}
}

// A set that replaces another spelling of the same env key (region, then
// REGION) takes the old spelling's link row with it. Without the carry-over a
// pinned value would reach every project under its new name.
func TestEnvSetOfAnotherSpellingKeepsTheReach(t *testing.T) {
	dir := envProject(t, "")
	other := t.TempDir()
	mustRun(t, dir, "airflow-variable", "set", "region", "--value", "us", "--global")
	mustRun(t, dir, "airflow-variable", "link", "region", other)

	mustRun(t, dir, "airflow-variable", "set", "REGION", "--value", "eu", "--global")
	if got := reachOf(t, "var:global:REGION"); got.Everywhere || !slices.Equal(got.Projects, []string{canonPath(t, other)}) {
		t.Errorf("REGION reach = %+v, want the pin carried over from region", got)
	}
	if got := reachOf(t, "var:global:region"); !got.Everywhere {
		t.Errorf("the replaced spelling's row survived: %+v", got)
	}
	if v := linkGetJSON(t, dir, "airflow-variable", "get", "REGION", "--global"); v.Value != "eu" || v.Reach == nil || v.Reach.AutoLink {
		t.Errorf("get = %+v", v)
	}
}

// With an index that cannot be read, which projects the replaced spelling
// reached is unknown, so the set is refused and nothing changes.
func TestEnvSetOfAnotherSpellingRefusesAnUnusableIndex(t *testing.T) {
	dir := envProject(t, "")
	mustRun(t, dir, "airflow-variable", "set", "region", "--value", "us", "--global")
	writeIndex(t, "{not json")
	_, _, err := run(t, dir, "airflow-variable", "set", "REGION", "--value", "eu", "--global")
	if err == nil || !strings.Contains(err.Error(), "links.idx") {
		t.Fatalf("err = %v, want a refusal naming the index", err)
	}
	if v := linkGetJSON(t, dir, "airflow-variable", "get", "region", "--global"); v.Value != "us" {
		t.Errorf("the refused set changed the value: %+v", v)
	}
}

// A plain global stays in the vault, so making a pinned global plain keeps its
// pin: nothing widens, and the value still resolves where it is linked.
func TestEnvPlainSetOfAPinnedGlobalKeepsThePin(t *testing.T) {
	dir := envProject(t, "")
	mustRun(t, dir, "variable", "set", "TOKEN", "--value", "x", "--global")
	mustRun(t, dir, "variable", "link", "TOKEN")
	mustRun(t, dir, "variable", "set", "TOKEN", "--value", "y", "--global", "--plain", "--replace-secret")
	if got := reachOf(t, "env:global:TOKEN"); got.Everywhere || len(got.Projects) != 1 {
		t.Errorf("reach = %+v, want the pin kept", got)
	}
	if v := linkGetJSON(t, dir, "variable", "get", "TOKEN"); v.Value != "y" || v.Source != vaultenv.SourceGlobal {
		t.Errorf("get = %+v, want the plain global where it is linked", v)
	}
}

// A new global starts out reaching no project: an empty row, written with the
// value, and a hint naming the link command. It is left out of list until
// linked, --all shows it, and a link brings it in.
func TestEnvSetGlobalCreatesItUnlinked(t *testing.T) {
	dir := envProject(t, "")
	_, stderr := mustRun(t, dir, "connection", "set", "warehouse", "--value", linkTestURI, "--global")
	if !strings.Contains(stderr, "warehouse reaches no project yet. Link it with `astro local env connection link warehouse`, or re-run with --auto-link.") {
		t.Errorf("stderr:\n%s", stderr)
	}
	if r := reachOf(t, "conn:global:warehouse"); r.Everywhere || len(r.Projects) != 0 {
		t.Fatalf("reach = %+v, want an empty row", r)
	}
	if out, _ := mustRun(t, dir, "connection", "list"); strings.Contains(out, "warehouse") {
		t.Errorf("list showed an unlinked global:\n%s", out)
	}
	if out, _ := mustRun(t, dir, "connection", "list", "--all"); !strings.Contains(out, "not linked here") {
		t.Errorf("list --all:\n%s", out)
	}
	if _, _, err := run(t, dir, "connection", "get", "warehouse"); err == nil {
		t.Error("an unlinked global resolved")
	}

	mustRun(t, dir, "connection", "link", "warehouse")
	if v := linkGetJSON(t, dir, "connection", "get", "warehouse"); v.Source != vaultenv.SourceGlobal {
		t.Errorf("after link, get = %+v", v)
	}
	if out, _ := mustRun(t, dir, "connection", "list"); !strings.Contains(out, "warehouse") {
		t.Errorf("a linked global is missing from list:\n%s", out)
	}
}

// --auto-link creates it with no row; updating an existing global, with or
// without the flag, never touches its row.
func TestEnvSetGlobalEverywhereAndUpdatesKeepTheRow(t *testing.T) {
	dir := envProject(t, "")
	mustRun(t, dir, "connection", "set", "open", "--value", linkTestURI, "--global", "--auto-link")
	if r := reachOf(t, "conn:global:open"); !r.Everywhere {
		t.Errorf("--auto-link wrote a row: %+v", r)
	}
	// An existing no-row global stays reaching every project on update.
	mustRun(t, dir, "connection", "set", "open", "--value", linkTestURI+"2", "--global")
	if r := reachOf(t, "conn:global:open"); !r.Everywhere {
		t.Errorf("an update narrowed an everywhere global: %+v", r)
	}
	// And an existing pinned one keeps its pin, --auto-link or not.
	mustRun(t, dir, "connection", "set", "pinned", "--value", linkTestURI, "--global")
	mustRun(t, dir, "connection", "link", "pinned")
	_, stderr := mustRun(t, dir, "connection", "set", "pinned", "--value", linkTestURI+"2", "--global", "--auto-link")
	if r := reachOf(t, "conn:global:pinned"); r.Everywhere || len(r.Projects) != 1 {
		t.Errorf("an update changed the row: %+v", r)
	}
	if !strings.Contains(stderr, "links were kept") {
		t.Errorf("stderr:\n%s", stderr)
	}
	if _, _, err := run(t, dir, "connection", "set", "p", "--value", linkTestURI, "--project", "--auto-link"); err == nil {
		t.Error("--auto-link on a project secret should be refused")
	}
}

// Creating a global needs its row; an index that cannot be written refuses
// the create, naming the file, and stores nothing.
func TestEnvSetGlobalRefusesAnUnusableIndex(t *testing.T) {
	dir := envProject(t, "")
	writeIndex(t, "{not json")
	_, _, err := run(t, dir, "connection", "set", "warehouse", "--value", linkTestURI, "--global")
	if err == nil || !strings.Contains(err.Error(), "links.idx") {
		t.Fatalf("err = %v, want a refusal naming the index", err)
	}
	if _, _, err := run(t, dir, "connection", "get", "warehouse", "--global"); err == nil {
		t.Error("the refused create stored a value")
	}
}
