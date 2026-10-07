// Package cliouttest pins the `--output json` contract: the schema goldens
// every command tree holds its published payloads against, and the switch
// that regenerates them.
//
// The payloads are the contract this CLI publishes. Scripts read them,
// agents read them, and Astro Desktop reads the same surface — so a field
// that quietly changes name or type breaks somebody who is not in this
// repository to notice. Each tree lists the types it publishes, and Check
// marshals each from a value with every field populated and compares it
// against a golden file, so renaming a field, dropping one, changing its
// type, or adding one shows up as a diff in the golden rather than as a
// surprise downstream. Adding a field is supposed to be easy — `make
// update-schemas` rewrites the goldens — but it is never silent.
//
// It lives beside cmd/cliout because that is the contract it pins, and
// because both trees that publish already import cliout: cmd/local and
// cmd/astro may not import each other, and a second copy in each would drift.
// It is test-only code outside a _test.go file, which an importable helper
// cannot avoid; scripts/deadcode.sh and .golangci.yml name it for that.
package cliouttest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
	"time"
	"unsafe"

	jsoncolor "github.com/neilotoole/jsoncolor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// UpdateEnv rewrites the goldens instead of comparing against them, when it
// is set to anything. `make update-schemas` sets it.
//
// An environment variable rather than a test flag, so one switch reaches
// every tree: `make update-schemas` runs one `go test` over several packages,
// and a custom flag given to that run is handed to every package's test
// binary, failing any that does not define it. An environment variable needs
// no package to declare anything.
const UpdateEnv = "ASTRO_UPDATE_SCHEMAS"

// Updating reports whether this run rewrites the goldens.
func Updating() bool {
	return os.Getenv(UpdateEnv) != ""
}

// Case is one published payload.
type Case struct {
	// Name is the golden file's stem, and reads as the surface it belongs
	// to rather than as the Go type, since the contract is the command's.
	Name string
	// Value is the payload's type, given as its zero value; Check populates
	// every field before it marshals.
	Value any
	// AsGiven marshals Value as it is instead of populating it. For a type
	// whose MarshalJSON publishes a different shape depending on a field —
	// deployment.VariableInfo's value is a string, or null for a secret —
	// populating can show only one branch, so the other is pinned as given.
	AsGiven bool
}

// GoldenPath is where c's golden lives under dir.
func GoldenPath(dir, name string) string {
	return filepath.Join(dir, name+".json")
}

// Check marshals c's payload, fully populated unless c.AsGiven, and holds it
// against its golden under dir — or rewrites the golden, under UpdateEnv.
func Check(t *testing.T, dir string, c Case) {
	t.Helper()

	value := c.Value
	if !c.AsGiven {
		v := reflect.New(reflect.TypeOf(c.Value))
		fill(t, v, 0)
		value = v.Elem().Interface()
	}
	got, err := json.MarshalIndent(value, "", "  ")
	require.NoError(t, err)
	got = append(got, '\n')

	path := GoldenPath(dir, c.Name)
	if Updating() {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, got, 0o600))
		return
	}

	want, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Fatalf("no pinned payload for %q.\n"+
			"A new `--output json` shape is a new public contract; run\n"+
			"  make update-schemas\n"+
			"and read the generated file before committing it.\n\ngot:\n%s", c.Name, got)
	}
	require.NoError(t, err)

	assert.Equal(t, string(want), string(got),
		"the %s payload changed shape.\n"+
			"Scripts, agents and Astro Desktop read this. If the change is\n"+
			"deliberate, run `make update-schemas` and say so\n"+
			"in the commit; if it is not, this is the bug.", c.Name)

	checkColorSafe(t, c.Name, value)
}

// checkColorSafe fails when coloring v on a terminal would change more than
// color. The colored encoder (jsoncolor) mangles some shapes plain encoding/json
// handles, such as maps with integer keys and fields tagged `,string`, writing
// escape codes inside the strings; a published type with one of those would
// break only on a terminal, where no test runs. Stripped of its color, the
// colored output must decode to the same value as the plain output. It need
// not match byte for byte: jsoncolor re-encodes a type's own MarshalJSON
// output with its keys sorted, which reorders keys and changes nothing a
// reader can rely on.
func checkColorSafe(t *testing.T, name string, v any) {
	t.Helper()
	var plain, colored bytes.Buffer
	enc := json.NewEncoder(&plain)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	require.NoError(t, enc.Encode(v))
	cenc := jsoncolor.NewEncoder(&colored)
	cenc.SetEscapeHTML(false)
	cenc.SetIndent("", "  ")
	cenc.SetColors(jsoncolor.DefaultColors())
	require.NoError(t, cenc.Encode(v))
	assert.JSONEq(t, plain.String(), ansiEscape.ReplaceAllString(colored.String(), ""),
		"the %s payload does not survive color on a terminal: jsoncolor writes it "+
			"differently from encoding/json. Change the type's shape (no integer map "+
			"keys, no `,string` tags) rather than this check.", name)
}

var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*m")

// goldenEntries is what dir holds. A tree that pins nothing yet has no
// directory of goldens, since an empty one cannot be committed, so a missing
// directory is an empty tree. Only then: a tree with cases has goldens, and a
// missing directory there is a wrong path, which fails t.
func goldenEntries(t testing.TB, dir string, hasCases bool) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) && !hasCases {
		return nil
	}
	require.NoError(t, err)
	return entries
}

// Orphans lists the goldens under dir that no case names.
//
// A golden nobody generates is a snapshot of a shape that may no longer
// exist — and e2e/schema_test.go looks goldens up by filename, so an orphan
// left behind by a renamed case would keep that test green against a dead
// file. Renaming a case writes the new golden and leaves the old one; this is
// what says so.
//
// A missing directory is read as goldenEntries reads it.
func Orphans(t testing.TB, dir string, cases []Case) []string {
	t.Helper()
	expected := map[string]bool{}
	for _, c := range cases {
		expected[c.Name+".json"] = true
	}

	entries := goldenEntries(t, dir, len(cases) > 0)
	var orphans []string
	for _, e := range entries {
		if !e.IsDir() && !expected[e.Name()] {
			orphans = append(orphans, e.Name())
		}
	}
	return orphans
}

// fixedTime keeps the goldens stable. Any real timestamp would rewrite them
// on every run.
var fixedTime = time.Date(2026, 7, 21, 10, 30, 0, 0, time.UTC)

// maxFillDepth bounds the walk. Reaching it means a payload nested deeper
// than anything here does today, or a type that refers to itself — either
// way the filler cannot honestly claim to have populated it, so it fails
// rather than writing a truncated shape into a golden that then reads as
// coverage.
const maxFillDepth = 14

// fill populates every field reachable from v, which must be addressable, so
// that the marshaled shape shows fields an `omitempty` zero value would hide.
// Populated deliberately rather than zero-valued: a contract test that cannot
// see half the contract is worse than none, because it reads as coverage.
//
// Values are chosen per kind rather than per field: the point is the shape,
// not the content. Signed, unsigned and floating point get visibly different
// sentinels, because JSON does not distinguish an int from a uint but it does
// distinguish 1 from 1.5 — and a field changing from float64 to int is a
// change to what a consumer can parse.
func fill(t *testing.T, v reflect.Value, depth int) {
	t.Helper()
	if depth > maxFillDepth {
		t.Fatalf("gave up filling at depth %d on a %s: either a payload nested "+
			"deeper than maxFillDepth, or a type that refers to itself. A "+
			"truncated golden is worse than none, so this fails instead.",
			depth, v.Type())
	}
	switch v.Kind() { //nolint:exhaustive // the kinds a JSON payload can hold
	case reflect.Pointer:
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		fill(t, v.Elem(), depth+1)
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Time{}) {
			v.Set(reflect.ValueOf(fixedTime))
			return
		}
		for i := range v.NumField() {
			fill(t, settableField(t, v, i), depth+1)
		}
	case reflect.Slice:
		// One element, itself filled: an empty slice would marshal as [] and
		// say nothing about what it holds.
		elem := reflect.New(v.Type().Elem()).Elem()
		fill(t, elem, depth+1)
		v.Set(reflect.Append(reflect.MakeSlice(v.Type(), 0, 1), elem))
	case reflect.Map:
		key := reflect.New(v.Type().Key()).Elem()
		fill(t, key, depth+1)
		val := reflect.New(v.Type().Elem()).Elem()
		fill(t, val, depth+1)
		m := reflect.MakeMap(v.Type())
		m.SetMapIndex(key, val)
		v.Set(m)
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(-1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(2)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1.5)
	case reflect.Interface:
		// Nothing meaningful to put behind an interface; leave it nil so the
		// golden records that the field is there and untyped.
	case reflect.Invalid:
		// A field this walk decided to skip. Nothing to do.
	}
}

// settableField returns field i of v in a form fill can write to, or the
// zero Value for a field that never reaches the wire.
//
// An embedded unexported struct type — `type connectionRow struct {
// connectionListRow; ... }` — is reported as an unexported field, so
// reflection refuses to set it. encoding/json promotes and marshals its
// exported fields regardless. Skipping it, which is what the obvious
// IsExported check does, left three goldens pinning a fraction of the
// payload they named: connection-row recorded 2 keys of 8, run-triggered 4
// of 13, and their embedded fields sat at "" in a file whose whole premise
// is that every field is populated.
//
// So the embedded case is re-derived through its address, which is what
// makes it settable. Ordinary unexported fields are skipped, because those
// genuinely do not reach the wire.
func settableField(t *testing.T, v reflect.Value, i int) reflect.Value {
	t.Helper()
	f := v.Field(i)
	if f.CanSet() {
		return f
	}
	sf := v.Type().Field(i)
	if !sf.Anonymous || f.Kind() != reflect.Struct || !f.CanAddr() {
		return reflect.Value{}
	}
	return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
}

// maxUnwrap bounds PayloadType's walk. `type Loop []Loop` and `type P *P` are
// legal Go, and Elem() on either returns the same type forever — a hang with
// a reflect stack rather than a diagnostic. fill bounds the same hazard for
// the same reason.
const maxUnwrap = 10

// PayloadType returns the shape behind a value handed to Emit: the element
// of a slice or array, the target of a pointer, the struct itself otherwise.
// nil means "not a shape a golden judges": a map, a bare string, an
// interface. A map IS a legal payload shape and nothing here covers one; it
// is a hole, not a judgement that maps are fine.
//
// A test that arms cliout.EmitObserver keys its tally by this, and so must
// the set of pinned types it compares against — pin something as
// &scaffold.Result{} and a raw reflect.TypeOf would miss it.
func PayloadType(t reflect.Type) reflect.Type {
	for range maxUnwrap {
		switch t.Kind() { //nolint:exhaustive // only the wrappers matter
		case reflect.Pointer, reflect.Slice, reflect.Array:
			if t.Elem() == t {
				return nil // self-referential; nothing to unwrap to
			}
			t = t.Elem()
		case reflect.Struct:
			return t
		default:
			return nil
		}
	}
	return nil
}
