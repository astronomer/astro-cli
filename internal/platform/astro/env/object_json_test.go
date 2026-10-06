package env

import (
	"bytes"
	"encoding/json"
	"reflect"
	"regexp"
	"strings"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

// ObjectInfo is the API's EnvironmentObject under snake_case keys and
// nothing else: the same fields, the same values, and the same fields absent.
// The API object is marshaled, its keys snake_cased, and held against what
// -o json publishes, once with every field set and once with none, so a
// field the API model gains, or the copy drops, or an optional field that
// starts appearing when unset, fails here.
func (s *Suite) TestObjectInfoIsTheAPIObjectInSnakeCase() {
	full := astrov1.EnvironmentObject{}
	fillValue(reflect.ValueOf(&full).Elem())
	for name, o := range map[string]*astrov1.EnvironmentObject{
		"every field set": &full,
		"no field set":    {},
	} {
		api, err := json.Marshal(o)
		s.Require().NoError(err)
		var want any
		s.Require().NoError(json.Unmarshal(api, &want))
		want = snakeKeys(want)

		var out bytes.Buffer
		s.Require().NoError(WriteConn(o, FormatJSON, &out))
		var got any
		s.Require().NoError(json.Unmarshal(out.Bytes(), &got))

		s.Equal(want, got, name)
	}
}

var camelHump = regexp.MustCompile(`([a-z0-9])([A-Z])`)

// snakeKeys renames every object key the way the published types spell
// them. The data maps (a connection's extra, an export's headers and labels)
// are filled with the key "x" here, which renaming leaves alone.
func snakeKeys(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[strings.ToLower(camelHump.ReplaceAllString(k, "${1}_${2}"))] = snakeKeys(e)
		}
		return out
	case []any:
		for i := range t {
			t[i] = snakeKeys(t[i])
		}
	}
	return v
}

// fillValue sets every field reachable from v, a pointer, slice and map
// holding one element, so that nothing marshals as absent.
func fillValue(v reflect.Value) {
	switch v.Kind() { //nolint:exhaustive // the kinds an API model holds
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		fillValue(v.Elem())
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Field(i).CanSet() {
				fillValue(v.Field(i))
			}
		}
	case reflect.Slice:
		e := reflect.New(v.Type().Elem()).Elem()
		fillValue(e)
		v.Set(reflect.Append(reflect.MakeSlice(v.Type(), 0, 1), e))
	case reflect.Map:
		e := reflect.New(v.Type().Elem()).Elem()
		fillValue(e)
		v.Set(reflect.MakeMap(v.Type()))
		v.SetMapIndex(reflect.ValueOf("x"), e)
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int:
		v.SetInt(5432)
	}
}
