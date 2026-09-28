package tomledit

import (
	"bytes"
	"reflect"
	"slices"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
	"github.com/pelletier/go-toml/v2/unstable/edit"
)

// lineIndent is what uv indents an array element with.
const lineIndent = "    "

// arrayValue is an array written as the value of a key-value line: the
// offsets of its brackets and of each element.
type arrayValue struct {
	open, close int
	elems       [][2]int
}

// multiline reports whether the array spans more than one line.
func (a arrayValue) multiline(data []byte) bool {
	return bytes.IndexByte(data[a.open:a.close], '\n') >= 0
}

// findArray locates the array at key when it is the value of a key-value
// line whose elements are all scalars. Anything else, an array inside an
// inline value or one holding tables or arrays, reports false.
func findArray(data []byte, key []string) (arrayValue, bool) {
	p := &unstable.Parser{}
	p.Reset(data)
	var table []string
	inArrayTable := false
	for p.NextExpression() {
		e := p.Expression()
		switch e.Kind { //nolint:exhaustive // only these kinds reach expression level without comments
		case unstable.Table:
			table, _ = headerAt(data, e)
			inArrayTable = false
		case unstable.ArrayTable:
			inArrayTable = true
		case unstable.KeyValue:
			parts, _ := headerAt(data, e)
			if inArrayTable || !slices.Equal(append(slices.Clone(table), parts...), key) {
				continue
			}
			return arrayOf(data, e)
		}
	}
	return arrayValue{}, false
}

func arrayOf(data []byte, kv *unstable.Node) (arrayValue, bool) {
	v := kv.Value()
	if v.Kind != unstable.Array {
		return arrayValue{}, false
	}
	end := int(kv.Raw.Offset + kv.Raw.Length)
	if end < 1 || end > len(data) || data[end-1] != ']' {
		return arrayValue{}, false
	}
	a := arrayValue{close: end - 1}
	it := kv.Key()
	for it.Next() {
		a.open = int(it.Node().Raw.Offset + it.Node().Raw.Length)
	}
	a.open += bytes.IndexByte(data[a.open:], '[')
	children := v.Children()
	for children.Next() {
		c := children.Node()
		if c.Raw.Length == 0 {
			return arrayValue{}, false
		}
		a.elems = append(a.elems, [2]int{int(c.Raw.Offset), int(c.Raw.Offset + c.Raw.Length)})
	}
	return a, true
}

// setLines is Set for an array of scalars at key, written one element per
// line with a trailing comma, the way uv writes dependencies. An array that
// lands inside an inline value stays on one line. It reports false, having
// done nothing, when the value is not such an array or is the one already
// there, which a plain Set leaves as it is spelled.
func (s *surgical) setLines(key []string, value any) (bool, error) {
	list, ok := value.([]any)
	if !ok || len(list) == 0 || slices.ContainsFunc(list, composite) {
		return false, nil
	}
	if cur, has := s.doc.Get(key); has && reflect.DeepEqual(cur, value) {
		return false, nil
	}
	if err := s.doc.Set(key, value); err != nil {
		return true, err
	}
	data := s.doc.Bytes()
	a, ok := findArray(data, key)
	if !ok {
		return true, nil
	}
	return true, s.splice(a.open, a.close+1, linesOf(data, a.elems, nil, eol(data)))
}

// appendLine is Set for index idx of the array at key when idx appends a
// scalar to an array written on a key-value line. A multi-line array gains
// one line after its last element, so the comments between its elements stay
// where they are; an array on one line is rewritten one element per line,
// its elements as they were spelled. It reports false when it does not apply.
func (s *surgical) appendLine(key []string, idx int, value any) (bool, error) {
	if composite(value) {
		return false, nil
	}
	data := s.doc.Bytes()
	a, ok := findArray(data, key)
	if !ok || idx != len(a.elems) {
		return false, nil
	}
	elem, err := renderScalar(value)
	if err != nil {
		return true, err
	}
	newline := eol(data)
	if !a.multiline(data) {
		return true, s.splice(a.open, a.close+1, linesOf(data, a.elems, elem, newline))
	}
	if len(a.elems) == 0 {
		return true, s.splice(a.open+1, a.open+1, slices.Concat(newline, []byte(lineIndent), elem, []byte(",")))
	}
	last := a.elems[len(a.elems)-1]
	var comma []byte
	after := skipTrivia(data, last[1], a.close)
	if data[after] != ',' {
		comma = []byte(",")
		after = last[1]
	}
	at := min(lineContentEnd(data, after), a.close)
	line := slices.Concat(newline, []byte(indentOf(data, last[0])), elem, []byte(","))
	if at == a.close {
		line = append(line, newline...)
	}
	var buf bytes.Buffer
	buf.Write(data[:last[1]])
	buf.Write(comma)
	buf.Write(data[last[1]:at])
	buf.Write(line)
	buf.Write(data[at:])
	return true, s.reparse(buf.Bytes())
}

// linesOf renders an array one element per line: the elements as data spells
// them, then extra when it is not nil.
func linesOf(data []byte, elems [][2]int, extra, newline []byte) []byte {
	out := []byte{'['}
	for _, e := range elems {
		out = slices.Concat(out, newline, []byte(lineIndent), data[e[0]:e[1]], []byte(","))
	}
	if extra != nil {
		out = slices.Concat(out, newline, []byte(lineIndent), extra, []byte(","))
	}
	return slices.Concat(out, newline, []byte("]"))
}

// indentOf is the whitespace before the element at off when it starts its
// line, and lineIndent when something else comes first on that line.
func indentOf(data []byte, off int) string {
	start := lineStart(data, off)
	lead := string(data[start:off])
	if strings.TrimLeft(lead, " \t") != "" {
		return lineIndent
	}
	return lead
}

// skipTrivia is the first offset from i, and before end, that is not
// whitespace, a line ending or a comment: what TOML allows between an array
// element and its comma.
func skipTrivia(data []byte, i, end int) int {
	for i < end {
		switch data[i] {
		case ' ', '\t', '\r', '\n':
			i++
		case '#':
			i = lineContentEnd(data, i)
		default:
			return i
		}
	}
	return i
}

// lineContentEnd is the end of the line holding off, before its line ending.
func lineContentEnd(data []byte, off int) int {
	end := lineEnd(data, off)
	if end > off && data[end-1] == '\n' {
		end--
		if end > off && data[end-1] == '\r' {
			end--
		}
	}
	return end
}

func (s *surgical) splice(from, to int, text []byte) error {
	data := s.doc.Bytes()
	return s.reparse(slices.Concat(data[:from], text, data[to:]))
}

func (s *surgical) reparse(data []byte) error {
	doc, err := edit.Parse(data)
	if err != nil {
		return &ParseError{Err: err}
	}
	s.doc = doc
	return nil
}

func composite(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return true
	}
	return false
}

// renderScalar renders one value the way Set would.
func renderScalar(v any) ([]byte, error) {
	line, err := toml.Marshal(map[string]any{"v": v})
	if err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(bytes.TrimPrefix(line, []byte("v = ")), []byte("\n")), nil
}
