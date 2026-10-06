package texttable

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func render(t *testing.T, tab *Table) string {
	t.Helper()
	var out bytes.Buffer
	b := bufio.NewWriter(&out)
	tab.Render(b)
	require.NoError(t, b.Flush())
	return out.String()
}

// The layout the CLI's tables have always had (printutil.Table's): a leading
// space, and every column, the last included, as wide as its widest cell plus
// five spaces. Pinned byte for byte, because it is the helper's whole claim.
func TestTableKeepsTheCLIsLayout(t *testing.T) {
	tab := &Table{Header: []string{"#", "KEY", "VALUE"}}
	tab.AddRow("1", "A", "1")
	tab.AddRow("2", "LONGER_KEY", "")
	assert.Equal(t, ""+
		" #     KEY            VALUE     \n"+
		" 1     A              1         \n"+
		" 2     LONGER_KEY               \n",
		render(t, tab))
}

func TestTableWithNoRows(t *testing.T) {
	assert.Equal(t, "Nothing here\n", render(t, &Table{Header: []string{"A"}, Empty: "Nothing here"}),
		"Empty replaces the table")
	assert.Equal(t, " A     B     \n", render(t, &Table{Header: []string{"A", "B"}}),
		"with no Empty, the header alone")
}

// A tab or a line break in a cell would open a column or split the row; each
// becomes a space, so every row stays one line with its columns in place.
func TestTableKeepsACellOnOneLine(t *testing.T) {
	tab := &Table{Header: []string{"K", "V", "S"}}
	tab.AddRow("a", "x\ty", "true")
	tab.AddRow("b", "one\r\ntwo", "false")
	// tabwriter also ends a cell at \v and a line at \f.
	tab.AddRow("c", "v\vw", "yes")
	tab.AddRow("d", "f\fg", "no")
	lines := strings.Split(strings.TrimSuffix(render(t, tab), "\n"), "\n")
	require.Len(t, lines, 5)
	assert.Equal(t, []string{"a", "x", "y", "true"}, strings.Fields(lines[1]))
	assert.Equal(t, []string{"b", "one", "two", "false"}, strings.Fields(lines[2]))
	assert.Equal(t, []string{"c", "v", "w", "yes"}, strings.Fields(lines[3]))
	assert.Equal(t, []string{"d", "f", "g", "no"}, strings.Fields(lines[4]))
	for i, last := range []string{"true", "false", "yes", "no"} {
		assert.Equal(t, strings.Index(lines[0], "S"), strings.Index(lines[i+1], last), "row %d", i+1)
	}
}

// Widths count characters: a column after an accented cell starts where it
// does in the other rows.
func TestTableAlignsByCharacter(t *testing.T) {
	tab := &Table{Header: []string{"K", "V", "S"}}
	tab.AddRow("a", "ééééé", "x")
	tab.AddRow("b", "e", "y")
	lines := strings.Split(render(t, tab), "\n")
	col := func(line, cell string) int {
		before, _, found := strings.Cut(line, cell)
		require.True(t, found, "no %q in %q", cell, line)
		return utf8.RuneCountInString(before)
	}
	assert.Equal(t, col(lines[0], "S"), col(lines[1], "x"))
	assert.Equal(t, col(lines[0], "S"), col(lines[2], "y"))
}
