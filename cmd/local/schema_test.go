package local

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The `--output json` payloads are the contract this CLI publishes. Scripts
// read them, agents read them, and Astro Desktop reads the same surface — so
// a field that quietly changes name or type breaks somebody who is not in
// this repository to notice.
//
// Nothing pinned them before. Every payload here is marshaled from a value
// with every field populated and compared against a golden file, so renaming
// a field, dropping one, changing its type, or adding one shows up as a diff
// in the golden rather than as a surprise downstream. Adding a field is
// supposed to be easy — `make update-schemas` rewrites the
// goldens — but it is never silent.
//
// Populated deliberately rather than zero-valued: `omitempty` hides a zero
// field, and a contract test that cannot see half the contract is worse than
// none, because it reads as coverage.

var updateSchemas = flag.Bool("update-schemas", false, "rewrite the golden JSON payload files")

// fixedTime keeps the goldens stable. Any real timestamp would rewrite them
// on every run.
var fixedTime = time.Date(2026, 7, 21, 10, 30, 0, 0, time.UTC)

// maxFillDepth bounds the walk. Reaching it means a payload nested deeper
// than anything here does today, or a type that refers to itself — either
// way the filler cannot honestly claim to have populated it, so it fails
// rather than writing a truncated shape into a golden that then reads as
// coverage.
const maxFillDepth = 12

// fill populates every field reachable from v, which must be addressable, so
// that the marshaled shape shows fields an `omitempty` zero value would hide.
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

// schemaCase is one published payload.
type schemaCase struct {
	// name is the golden file's stem, and reads as the surface it belongs
	// to rather than as the Go type, since the contract is the command's.
	name  string
	value any
}

func goldenPath(name string) string {
	return filepath.Join("testdata", "schema", name+".json")
}

// checkSchema marshals a fully-populated payload and holds it against the
// golden.
func checkSchema(t *testing.T, c schemaCase) {
	t.Helper()

	v := reflect.New(reflect.TypeOf(c.value))
	fill(t, v, 0)
	got, err := json.MarshalIndent(v.Elem().Interface(), "", "  ")
	require.NoError(t, err)
	got = append(got, '\n')

	path := goldenPath(c.name)
	if *updateSchemas {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, got, 0o600))
		return
	}

	want, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Fatalf("no pinned payload for %q.\n"+
			"A new `--output json` shape is a new public contract; run\n"+
			"  make update-schemas\n"+
			"and read the generated file before committing it.\n\ngot:\n%s", c.name, got)
	}
	require.NoError(t, err)

	assert.Equal(t, string(want), string(got),
		"the %s payload changed shape.\n"+
			"Scripts, agents and Astro Desktop read this. If the change is\n"+
			"deliberate, run `make update-schemas` and say so\n"+
			"in the commit; if it is not, this is the bug.", c.name)
}
