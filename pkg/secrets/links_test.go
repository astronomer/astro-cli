package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
)

// absPath is an absolute, clean path on the platform running the test, for
// link rows that must pass checkScope.
func absPath(t *testing.T, parts ...string) string {
	t.Helper()
	root := "/"
	if runtime.GOOS == "windows" {
		root = `C:\`
	}
	return filepath.Join(append([]string{root}, parts...)...)
}

func writeLinks(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(LinksPath(dir), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestNoIndexReachesEveryProject(t *testing.T) {
	l, err := OpenLinks(filepath.Join(t.TempDir(), "never-created"))
	if err != nil {
		t.Fatalf("a missing index is the ordinary state, not an error: %v", err)
	}
	r := l.ReachOf("conn:global:warehouse")
	if !r.Everywhere {
		t.Errorf("ReachOf with no index = %+v, want Everywhere", r)
	}
	if !r.Includes(Checkout{}) {
		t.Error("a no-row entry must reach even a directory outside any project")
	}
	var nilLinks *Links
	if !nilLinks.ReachOf("conn:global:warehouse").Everywhere {
		t.Error("a nil *Links must read as no rows")
	}
}

func TestIndexRowsRestrictReach(t *testing.T) {
	dir := t.TempDir()
	etl, other := absPath(t, "work", "etl"), absPath(t, "work", "other")
	body := fmt.Sprintf(`{"version":1,"links":{
		"conn:global:warehouse":{"projects":[%q]},
		"env:global:SLACK_TOKEN":{"projects":[]},
		"var:global:NULLED":{"projects":null},
		"var:global:BARE":{}}}`, etl)
	writeLinks(t, dir, body)
	l, err := OpenLinks(dir)
	if err != nil {
		t.Fatal(err)
	}

	wh := l.ReachOf("conn:global:warehouse")
	if wh.Everywhere || !wh.Includes(Checkout{Path: etl}) || wh.Includes(Checkout{Path: other}) {
		t.Errorf("warehouse reach = %+v: want only %s", wh, etl)
	}
	// A present row that names nothing is "no project", in every spelling. The
	// unsafe reading, "every project", is the mutant this guards.
	for _, key := range []string{"env:global:SLACK_TOKEN", "var:global:NULLED", "var:global:BARE"} {
		r := l.ReachOf(key)
		if r.Everywhere || r.Includes(Checkout{Path: etl, Home: etl}) {
			t.Errorf("%s reach = %+v, want no project", key, r)
		}
	}
	if !l.ReachOf("env:global:NOT_LISTED").Everywhere {
		t.Error("a key with no row must reach every project")
	}
}

func TestIncludesMatchesPathOrHome(t *testing.T) {
	main, wt, sub := absPath(t, "repo"), absPath(t, "wt", "feature"), absPath(t, "repo", "sub")
	r := Reach{Projects: []string{main}}
	cases := []struct {
		name string
		c    Checkout
		want bool
	}{
		{"the project itself", Checkout{Path: main, Home: main}, true},
		{"a worktree of it, by home", Checkout{Path: wt, Home: main}, true},
		{"a subdirectory is not the project", Checkout{Path: sub, Home: sub}, false},
		{"no checkout", Checkout{}, false},
		{"home only", Checkout{Home: main}, true},
	}
	for _, tc := range cases {
		if got := r.Includes(tc.c); got != tc.want {
			t.Errorf("%s: Includes(%+v) = %v, want %v", tc.name, tc.c, got, tc.want)
		}
	}
	// And a link naming one worktree reaches that worktree and not the project.
	one := Reach{Projects: []string{wt}}
	if !one.Includes(Checkout{Path: wt, Home: main}) || one.Includes(Checkout{Path: main, Home: main}) {
		t.Error("a link to one worktree must reach it and only it")
	}
	// Empty strings never match, even against an empty checkout half.
	if (Reach{Projects: []string{""}}).Includes(Checkout{}) {
		t.Error(`an empty path matched an empty checkout`)
	}
}

func TestUnreadableAndNewerIndexesFail(t *testing.T) {
	cases := []struct {
		name, body string
		want       error
	}{
		{"corrupt", `{"version":1,"links":`, ErrLinksUnreadable},
		{"not an object", `[]`, ErrLinksUnreadable},
		{"no version", `{"links":{}}`, ErrLinksUnreadable},
		{"a row that is not an object", `{"version":1,"links":{"env:global:A":true}}`, ErrLinksUnreadable},
		{"newer", `{"version":2,"links":{}}`, ErrLinksTooNew},
	}
	for _, tc := range cases {
		dir := t.TempDir()
		writeLinks(t, dir, tc.body)
		l, err := OpenLinks(dir)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: OpenLinks err = %v, want %v", tc.name, err, tc.want)
		}
		if l != nil {
			t.Errorf("%s: OpenLinks returned an index alongside the error", tc.name)
		}
		if err != nil && !strings.Contains(err.Error(), "links.idx") {
			t.Errorf("%s: err = %q, want it to name the file", tc.name, err)
		}
		// Writers refuse too, and leave the file as they found it.
		uerr := UpdateLinks(dir, func(m map[string]Reach) error {
			m["env:global:A"] = Reach{Projects: []string{}}
			return nil
		})
		if !errors.Is(uerr, tc.want) {
			t.Errorf("%s: UpdateLinks err = %v, want %v", tc.name, uerr, tc.want)
		}
		if got, _ := os.ReadFile(LinksPath(dir)); string(got) != tc.body { // compared below
			t.Errorf("%s: UpdateLinks rewrote a file it refused: %s", tc.name, got)
		}
	}
}

// A path that could not be a project path on any platform is dropped on read,
// which narrows the row. It never widens it, and "global" is not a path.
func TestImplausiblePathsNarrowARow(t *testing.T) {
	dir := t.TempDir()
	writeLinks(t, dir, `{"version":1,"links":{"env:global:A":{"projects":["relative/p","global","/ok"]}}}`)
	l, err := OpenLinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	r := l.ReachOf("env:global:A")
	if r.Everywhere || !slices.Equal(r.Projects, []string{"/ok"}) {
		t.Errorf("reach = %+v, want only /ok", r)
	}
}

func TestUpdateLinksRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "secrets") // created by the first write
	etl, other := absPath(t, "work", "etl"), absPath(t, "work", "other")

	err := UpdateLinks(dir, func(m map[string]Reach) error {
		if len(m) != 0 {
			t.Errorf("a fresh index handed fn %v", m)
		}
		m["conn:global:warehouse"] = Reach{Projects: []string{other, etl, etl}}
		m["env:global:SLACK_TOKEN"] = Reach{}
		m["var:global:X"] = Reach{Everywhere: true} // same as no row
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(LinksPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("index mode = %v, want 0600", info.Mode().Perm())
	}

	l, err := OpenLinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := l.ReachOf("conn:global:warehouse").Projects; !slices.Equal(got, []string{etl, other}) {
		t.Errorf("warehouse = %v, want sorted and deduplicated", got)
	}
	if r := l.ReachOf("env:global:SLACK_TOKEN"); r.Everywhere || len(r.Projects) != 0 {
		t.Errorf("SLACK_TOKEN = %+v, want no project", r)
	}
	if !l.ReachOf("var:global:X").Everywhere {
		t.Error("Everywhere must be written as no row")
	}
	raw, _ := os.ReadFile(LinksPath(dir)) // checked by content
	if !strings.Contains(string(raw), `"projects": []`) {
		t.Errorf("an empty reach must be written as [], not null or absent:\n%s", raw)
	}

	// Deleting a key removes its row.
	if err := UpdateLinks(dir, func(m map[string]Reach) error {
		delete(m, "conn:global:warehouse")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if l, _ = OpenLinks(dir); !l.ReachOf("conn:global:warehouse").Everywhere { // a nil l would panic the test, which is fine
		t.Error("a deleted row still restricts reach")
	}
}

// A newer peer's additive fields survive a rewrite, at the top level and in a
// row, whether the row was edited or left alone.
func TestUpdateLinksKeepsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	p := absPath(t, "p")
	body := fmt.Sprintf(`{"version":1,"written_by":"desktop 9.9","links":{
		"env:global:EDITED":{"projects":[],"note":"keep me"},
		"env:global:ALONE":{"projects":[%q],"since":"2026-09-27"},
		"future:global:KIND":{"projects":[]}}}`, p)
	writeLinks(t, dir, body)

	if err := UpdateLinks(dir, func(m map[string]Reach) error {
		m["env:global:EDITED"] = Reach{Projects: []string{p}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version   int                        `json:"version"`
		WrittenBy string                     `json:"written_by"`
		Links     map[string]json.RawMessage `json:"links"`
	}
	raw, err := os.ReadFile(LinksPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Version != 1 || doc.WrittenBy != "desktop 9.9" {
		t.Errorf("top level lost a field: %s", raw)
	}
	for key, field := range map[string]string{"env:global:EDITED": `"note"`, "env:global:ALONE": `"since"`} {
		if !strings.Contains(string(doc.Links[key]), field) {
			t.Errorf("row %s lost %s: %s", key, field, doc.Links[key])
		}
	}
	if _, ok := doc.Links["future:global:KIND"]; !ok {
		t.Error("a row with a kind this build does not know was dropped")
	}
}

func TestUpdateLinksValidatesWhatItWrites(t *testing.T) {
	good := absPath(t, "p")
	cases := []struct {
		name string
		key  string
		path string
	}{
		{"project-scoped key", "env:" + good + ":A", good},
		{"malformed key", "not-a-key", good},
		{"relative path", "env:global:A", filepath.Join("rel", "p")},
		{"unclean path", "env:global:A", good + string(filepath.Separator) + "." + string(filepath.Separator) + "x"},
		{"global as a path", "env:global:A", GlobalScope},
	}
	for _, tc := range cases {
		dir := t.TempDir()
		err := UpdateLinks(dir, func(m map[string]Reach) error {
			m[tc.key] = Reach{Projects: []string{tc.path}}
			return nil
		})
		if !errors.Is(err, ErrBadKey) {
			t.Errorf("%s: err = %v, want ErrBadKey", tc.name, err)
		}
		if _, serr := os.Stat(LinksPath(dir)); !errors.Is(serr, os.ErrNotExist) {
			t.Errorf("%s: a refused write still published the index", tc.name)
		}
	}
	// fn's own error aborts the write.
	dir := t.TempDir()
	boom := errors.New("boom")
	if err := UpdateLinks(dir, func(m map[string]Reach) error {
		m["env:global:A"] = Reach{}
		return boom
	}); !errors.Is(err, boom) {
		t.Errorf("err = %v, want fn's error", err)
	}
	if _, serr := os.Stat(LinksPath(dir)); !errors.Is(serr, os.ErrNotExist) {
		t.Error("fn failed and the index was written anyway")
	}
}

// Neither file is a value file: a listing does not report them, and a vault
// holding only link state is still "never used" for key minting. Otherwise the
// first link written would make every later key creation refuse as orphaned.
func TestListMetaAndHasValuesSkipTheLinkFiles(t *testing.T) {
	dir := t.TempDir()
	if err := UpdateLinks(dir, func(m map[string]Reach) error {
		m["env:global:A"] = Reach{}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{linksFile, linksLock} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s not written: %v", name, err)
		}
	}
	s := testStore(t, newFakeKeyring(), "astro", dir)
	metas, err := s.ListMeta()
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 0 {
		t.Errorf("ListMeta = %v, want the link files skipped", metas)
	}
	has, err := s.hasValues()
	if err != nil || has {
		t.Errorf("hasValues = %v, %v; want false: link state is not an encrypted value", has, err)
	}
	if err := s.Set("env:global:A", "v"); err != nil {
		t.Errorf("first Set beside link state: %v", err)
	}
}

// OpenLinks reads a file and nothing else: no keyring, not even a store.
// Structural, but pinned so a future "validate keys against the vault" does not
// quietly add a keychain prompt to every listing.
func TestOpenLinksNeedsNoStore(t *testing.T) {
	dir := t.TempDir()
	writeLinks(t, dir, `{"version":1,"links":{"env:global:A":{"projects":[]}}}`)
	kr := newFakeKeyring()
	_ = testStore(t, kr, "astro", dir)
	if _, err := OpenLinks(dir); err != nil {
		t.Fatal(err)
	}
	if kr.gets+kr.sets != 0 {
		t.Errorf("keyring touched %d times", kr.gets+kr.sets)
	}
}

func TestUpdateLinksTimesOutOnAHeldLock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	unlock, err := fsatomic.Lock(filepath.Join(dir, linksLock))
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	start := time.Now()
	err = UpdateLinks(dir, func(map[string]Reach) error {
		t.Error("fn ran without the lock")
		return nil
	})
	if !errors.Is(err, fsatomic.ErrLockTimeout) {
		t.Fatalf("err = %v, want a lock timeout", err)
	}
	if !strings.Contains(err.Error(), "link index") || !strings.Contains(err.Error(), linksLock) {
		t.Errorf("err = %q, want it to say what was being locked and where", err)
	}
	if elapsed := time.Since(start); elapsed < fsatomic.LockTimeout {
		t.Errorf("gave up after %s, before the %s timeout", elapsed, fsatomic.LockTimeout)
	}
}

// helperEnv switches this test binary into a writer process for
// TestConcurrentProcessesDoNotLoseLinks.
const (
	helperDirEnv = "ASTRO_SECRETS_LINKS_HELPER_DIR"
	helperIDEnv  = "ASTRO_SECRETS_LINKS_HELPER_ID"
	helperKey    = "env:global:SHARED"
	helperWrites = 15
)

// TestLinksWriterProcess is not a test: it is the body of one writer process.
// Each write adds one path to the same row, and the hook stretches the window
// between read and publish, so without the lock two processes reading the same
// row and each publishing their own addition lose one of them every time.
func TestLinksWriterProcess(t *testing.T) {
	dir := os.Getenv(helperDirEnv)
	if dir == "" {
		t.Skip("helper process only")
	}
	id := os.Getenv(helperIDEnv)
	updateLinksHook = func() { time.Sleep(3 * time.Millisecond) }
	for i := range helperWrites {
		p := absPath(t, "p", id+"-"+strconv.Itoa(i))
		if err := UpdateLinks(dir, func(m map[string]Reach) error {
			r := m[helperKey]
			r.Projects = append(r.Projects, p)
			m[helperKey] = r
			return nil
		}); err != nil {
			t.Fatalf("writer %s: %v", id, err)
		}
	}
}

// Two processes (four, here) editing the index at once serialize on the lock
// and lose nothing, while a reader polling throughout sees an old file or a new
// one and never a torn one.
func TestConcurrentProcessesDoNotLoseLinks(t *testing.T) {
	if os.Getenv(helperDirEnv) != "" {
		t.Skip("inside a helper")
	}
	dir := t.TempDir()
	const procs = 4

	var stop atomic.Bool
	var reads atomic.Int64
	var readerDone sync.WaitGroup
	readerDone.Add(1)
	go func() {
		defer readerDone.Done()
		for !stop.Load() {
			if _, err := OpenLinks(dir); err != nil {
				t.Errorf("a reader during writes saw: %v", err)
			}
			reads.Add(1)
		}
	}()

	var wg sync.WaitGroup
	for i := range procs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestLinksWriterProcess$", "-test.count=1") // re-executing this test binary
			cmd.Env = append(os.Environ(), helperDirEnv+"="+dir, helperIDEnv+"="+strconv.Itoa(i))
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("writer %d: %v\n%s", i, err, out)
			}
		}()
	}
	wg.Wait()
	stop.Store(true)
	readerDone.Wait()

	l, err := OpenLinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := l.ReachOf(helperKey).Projects
	if want := procs * helperWrites; len(got) != want {
		t.Errorf("row holds %d paths, want %d: concurrent writers lost an edit", len(got), want)
	}
	if reads.Load() == 0 {
		t.Error("the reader never ran, so tearing was not tested")
	}
}
