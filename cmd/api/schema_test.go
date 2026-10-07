package api

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout/cliouttest"
)

// The shape `astro api airflow|cloud|registry describe --json` publishes,
// pinned by a golden in testdata/schema the way the command trees pin theirs
// (see cliouttest). cmd cannot pin it: it imports this package, and the
// describe types are unexported. `ls --json` publishes apirequest.EndpointList,
// the listing `astro local api ls` shares, and cmd/local pins it once, as
// api-endpoint-list. `make update-schemas` rewrites the golden; read the diff
// before committing it.

// schemaDir holds this package's goldens.
var schemaDir = filepath.Join("testdata", "schema")

// describeSample is describe's payload with every field of every type set
// once. Given as is, because schemaJSON nests itself and cannot be filled to
// a fixed depth.
func describeSample() describeOutput {
	leaf := func() *schemaJSON { return &schemaJSON{Type: "x"} }
	schema := &schemaJSON{
		Ref: "x", Circular: true, Truncated: true, Type: "x", Format: "x",
		Description: "x", Required: []string{"x"}, ReadOnly: true, Deprecated: true,
		Properties: []propertyJSON{{Name: "x", Schema: leaf()}},
		Items:      leaf(),
		OneOf:      []*schemaJSON{leaf()},
		AnyOf:      []*schemaJSON{leaf()},
		AllOf:      []*schemaJSON{leaf()},
		Enum:       []any{"x"},
		Default:    "x",
		Example:    "x",
	}
	return describeOutput{Count: 1, Endpoints: []endpointJSON{{
		Method: "x", Path: "x", OperationID: "x", Summary: "x", Description: "x",
		Deprecated: true, Tags: []string{"x"},
		Parameters:  []parameterJSON{{Name: "x", In: "x", Description: "x", Required: true, Schema: schema}},
		RequestBody: &requestBodyJSON{Description: "x", Required: true, Schema: leaf()},
		Responses:   []responseJSON{{Code: "x", Description: "x", Schema: leaf()}},
	}}}
}

var publishedPayloads = []cliouttest.Case{
	// describe --json: the matched endpoints under "endpoints", schemas
	// resolved.
	{Name: "api-describe", Value: describeSample(), AsGiven: true},
}

func TestPublishedJSONPayloadsKeepTheirShape(t *testing.T) {
	for _, c := range publishedPayloads {
		t.Run(c.Name, func(t *testing.T) { cliouttest.Check(t, schemaDir, c) })
	}
}

// describeSample is hand-built, so a field added to a describe type would be
// missing from the golden without this. It walks the sample by reflection, so
// every struct type reachable from it, including one added later, must have
// each of its fields set somewhere in the sample.
func TestDescribeSampleSetsEveryField(t *testing.T) {
	set := map[reflect.Type]map[int]bool{}
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		kind := v.Kind()
		if kind == reflect.Pointer || kind == reflect.Interface {
			if !v.IsNil() {
				walk(v.Elem())
			}
			return
		}
		if kind == reflect.Slice || kind == reflect.Array {
			for i := range v.Len() {
				walk(v.Index(i))
			}
			return
		}
		if kind != reflect.Struct {
			return
		}
		fields := set[v.Type()]
		if fields == nil {
			fields = map[int]bool{}
			set[v.Type()] = fields
		}
		for i := range v.NumField() {
			if !v.Field(i).IsZero() {
				fields[i] = true
			}
			walk(v.Field(i))
		}
	}
	walk(reflect.ValueOf(describeSample()))

	require.NotEmpty(t, set)
	for typ, fields := range set {
		for i := range typ.NumField() {
			assert.True(t, fields[i], "describeSample never sets %s.%s", typ.Name(), typ.Field(i).Name)
		}
	}
}

func TestEveryGoldenHasACase(t *testing.T) {
	assert.Empty(t, cliouttest.Orphans(t, schemaDir, publishedPayloads),
		"these goldens have no case in publishedPayloads; delete the file, or the one a renamed case left behind.")
}

// The keys describe publishes are ours, not the spec's: an OpenAPI field
// such as operationId or requestBody is renamed snake_case like every other
// published key. Values under enum, default and example are the spec's data;
// the sample sets them to scalars, so no spec key reaches the golden.
func TestPublishedKeysAreSnakeCase(t *testing.T) {
	assert.Empty(t, cliouttest.KeyProblems(t, schemaDir, len(publishedPayloads) > 0, nil),
		"give the field a snake_case json tag and run `make update-schemas`")
}
