package jsonstr

import (
	"fmt"
)

// MapToJsonObjStr converts an __unnested__ map into a str representation of a json object (unquoted keys)
func MapToJsonObjStr(m map[string]string) string {
	s := "{"
	for k, v := range m {
		s += fmt.Sprintf("%s:\"%s\"", k, v)
	}

	s += "}"
	return s
}
