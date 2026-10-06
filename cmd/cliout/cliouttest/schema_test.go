package cliouttest

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The goldens themselves are exercised by the trees that hold them (cmd/local
// and cmd/astro). What is tested here is what neither of them can see.

// A self-referential type must not hang the walk. `type Loop []Loop` is
// legal Go and Elem() on it returns Loop forever.
func TestPayloadTypeSurvivesASelfReferentialType(t *testing.T) {
	type Loop []Loop
	assert.Nil(t, PayloadType(reflect.TypeOf(Loop{})))
}

// A map is not judged here, and the doc says so rather than pretending maps
// are fine.
func TestPayloadTypeSkipsShapesItDoesNotJudge(t *testing.T) {
	assert.Nil(t, PayloadType(reflect.TypeOf(map[string]string{})))
	assert.Nil(t, PayloadType(reflect.TypeOf("")))
}

// AsGiven is the one way to pin a branch filling cannot reach, so it must
// marshal the value it was handed, zero fields included, and not a populated
// one.
func TestCheckMarshalsAnAsGivenValueUnfilled(t *testing.T) {
	type payload struct {
		Name   string `json:"name"`
		Secret bool   `json:"secret"`
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(GoldenPath(dir, "p"), []byte("{\n  \"name\": \"plain\",\n  \"secret\": false\n}\n"), 0o600))

	t.Setenv(UpdateEnv, "")
	Check(t, dir, Case{Name: "p", Value: payload{Name: "plain"}, AsGiven: true})
}

// Without AsGiven every field is populated, so the golden shows the fields an
// omitempty zero value would hide.
func TestCheckPopulatesEveryField(t *testing.T) {
	type payload struct {
		Name  string  `json:"name,omitempty"`
		Count int     `json:"count,omitempty"`
		Rate  float64 `json:"rate,omitempty"`
		Tags  []string
	}
	dir := t.TempDir()
	t.Setenv(UpdateEnv, "1")
	Check(t, dir, Case{Name: "p", Value: payload{}})

	got, err := os.ReadFile(filepath.Join(dir, "p.json"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"x","count":-1,"rate":1.5,"Tags":["x"]}`, string(got))
}

func TestOrphansNamesAGoldenNoCaseWrites(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"kept.json", "renamed-away.json"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("{}\n"), 0o600))
	}
	assert.Equal(t, []string{"renamed-away.json"}, Orphans(t, dir, []Case{{Name: "kept"}}))
}
