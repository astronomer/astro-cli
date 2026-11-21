package output

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A listing (deployments, workspaces, ...) piped is tab-separated with no
// color on the current row, and its empty message is not on stdout.
func TestPrintTableIsTabSeparatedWhenPiped(t *testing.T) {
	cfg := BuildTableConfig(
		[]Column[testItem]{
			{Header: "NAME", Value: func(i testItem) string { return i.Name }},
			{Header: "ID", Value: func(i testItem) string { return i.ID }},
		},
		func(d any) []testItem { return d.(*testList).Items },
		WithColorRow(func(i testItem) bool { return i.IsCurrent }, [2]string{"\033[1;32m", "\033[0m"}),
		WithNoResultsMsg("No items found"),
	)
	out, err := text(t, &testList{Items: []testItem{{Name: "alpha", ID: "id-1"}, {Name: "beta", ID: "id-2", IsCurrent: true}}}, cfg)
	require.NoError(t, err)
	assert.Equal(t, "NAME\tID\nalpha\tid-1\nbeta\tid-2\n", out)

	out, err = text(t, &testList{Items: []testItem{}}, cfg)
	require.NoError(t, err)
	assert.Equal(t, "NAME\tID\n", out)
}
