package printutil

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func pickTable() *Table {
	t := &Table{Padding: []int{5, 20}, DynamicPadding: true, Header: []string{"#", "NAME"}}
	t.AddRow([]string{"1", "dev"}, false)
	t.AddRow([]string{"2", "prod"}, false)
	return t
}

// Pick prints the title, the table and the prompt, and reads a row number.
func TestPickReadsARowNumber(t *testing.T) {
	var out bytes.Buffer
	i, ok := pickTable().Pick(&out, strings.NewReader("2\n"), "Select a thing")
	assert.True(t, ok)
	assert.Equal(t, 1, i)
	got := out.String()
	assert.True(t, strings.HasPrefix(got, "Select a thing\n"), got)
	assert.Regexp(t, `#\s+NAME`, got)
	assert.Regexp(t, `2\s+prod`, got)
	assert.True(t, strings.HasSuffix(got, "\n\n> "), "%q", got)

	out.Reset()
	_, ok = pickTable().Pick(&out, strings.NewReader("1\r\n"), "")
	assert.True(t, ok, "a CRLF answer")
	assert.True(t, strings.HasPrefix(out.String(), " #") || strings.HasPrefix(out.String(), "#"), "an empty title printed a line: %q", out.String())
}

// Anything but a row number picks nothing.
func TestPickRefusesAnythingButARowNumber(t *testing.T) {
	for _, answer := range []string{"", "\n", "0\n", "3\n", "prod\n", " 1\n", "+1\n"} {
		_, ok := pickTable().Pick(&bytes.Buffer{}, strings.NewReader(answer), "t")
		assert.False(t, ok, "answer %q", answer)
	}
}
