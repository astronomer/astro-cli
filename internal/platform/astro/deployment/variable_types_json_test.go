package deployment

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// value is always published: an empty plain value is "" and a secret is
// null, so `KEY=""` cannot be mistaken for a secret, and a script reading
// value never finds the key missing.
func TestVariableInfoJSONAlwaysCarriesValue(t *testing.T) {
	for _, c := range []struct {
		in   VariableInfo
		want string
	}{
		{VariableInfo{Key: "A", Value: "x"}, `{"key":"A","value":"x","is_secret":false}`},
		{VariableInfo{Key: "EMPTY", Value: ""}, `{"key":"EMPTY","value":"","is_secret":false}`},
		{VariableInfo{Key: "S", IsSecret: true}, `{"key":"S","value":null,"is_secret":true}`},
		// A secret never publishes a value, even if one was set by mistake.
		{VariableInfo{Key: "S2", Value: "leak", IsSecret: true}, `{"key":"S2","value":null,"is_secret":true}`},
	} {
		got, err := json.Marshal(c.in)
		require.NoError(t, err)
		assert.Equal(t, c.want, string(got), c.in.Key)
	}
}
