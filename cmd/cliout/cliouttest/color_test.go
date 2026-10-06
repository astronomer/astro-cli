package cliouttest

import (
	"bytes"
	"encoding/json"
	"testing"

	jsoncolor "github.com/neilotoole/jsoncolor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// checkColorSafe exists because jsoncolor really does mangle these shapes:
// stripped of its color, its output no longer decodes to the value
// encoding/json writes. If a jsoncolor upgrade fixes them, this test fails and
// the check can be relaxed; until then, a published type with either shape
// fails its golden case.
func TestJSONColorManglesTheShapesTheCheckGuards(t *testing.T) {
	for name, v := range map[string]any{
		"integer map keys": map[int]string{1: "one"},
		"string-tagged int": struct {
			N int `json:"n,string"`
		}{N: 1},
	} {
		var plain, colored bytes.Buffer
		require.NoError(t, json.NewEncoder(&plain).Encode(v), name)
		cenc := jsoncolor.NewEncoder(&colored)
		cenc.SetColors(jsoncolor.DefaultColors())
		require.NoError(t, cenc.Encode(v), name)
		stripped := ansiEscape.ReplaceAllString(colored.String(), "")
		assert.NotEqual(t, normalize(t, plain.String()), normalize(t, stripped),
			"%s: jsoncolor now encodes it like encoding/json", name)
	}
}

// normalize decodes s, or returns it as is when it is not json (a mangled
// encoding may not parse at all, which is just as unequal).
func normalize(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	return v
}
