package tomledit

import (
	"bytes"
	"slices"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
	"github.com/pelletier/go-toml/v2/unstable/edit"
)

// surgical is the comment-preserving implementation, over go-toml's
// unstable/edit document API.
type surgical struct {
	doc *edit.Document
}

// NewSurgical returns an Editor whose output is byte-identical to src
// except for the bytes each edit rewrites.
func NewSurgical(src []byte) (Editor, error) {
	doc, err := edit.Parse(src)
	if err != nil {
		return nil, &ParseError{Err: err}
	}
	return &surgical{doc: doc}, nil
}

func (s *surgical) Get(key []string) (any, bool) {
	return s.doc.Get(key)
}

func (s *surgical) Set(key []string, value any) error {
	if err := s.doc.Set(key, value); err != nil {
		return &KeyError{Key: key, Err: err}
	}
	return nil
}

func (s *surgical) EnsureTablesAtTop(keys [][]string) error {
	data := s.doc.Bytes()
	newline := eol(data)
	var text []byte
	for _, key := range keys {
		if len(key) == 0 {
			return &KeyError{Key: key, Reason: "empty key"}
		}
		if _, ok := s.doc.Get(key); ok {
			continue
		}
		header, err := headerKey(key)
		if err != nil {
			return &KeyError{Key: key, Err: err}
		}
		if len(text) > 0 {
			text = append(text, newline...)
		}
		text = append(text, "["+header+"]"...)
		text = append(text, newline...)
	}
	if len(text) == 0 {
		return nil
	}
	at, err := topInsertAt(data, keys)
	if err != nil {
		return &ParseError{Err: err}
	}

	var buf bytes.Buffer
	buf.Write(data[:at])
	// Only a document with no final newline ends outside a line boundary.
	freshLine := at == 0 || data[at-1] == '\n'
	if !freshLine {
		buf.Write(newline)
	}
	if at > 0 && (!freshLine || !blankLineBefore(data, at)) {
		buf.Write(newline)
	}
	buf.Write(text)
	if at < len(data) {
		buf.Write(newline)
	}
	buf.Write(data[at:])

	doc, err := edit.Parse(buf.Bytes())
	if err != nil {
		return &ParseError{Err: err}
	}
	s.doc = doc
	return nil
}

func (s *surgical) Delete(key []string) bool {
	return s.doc.Delete(key)
}

func (s *surgical) Bytes() ([]byte, error) {
	return s.doc.Bytes(), nil
}

// topInsertAt returns where the tables of keys belong: the start of the first
// table header the document has that keys does not name, comments attached to
// that header included. Everything above that point stays above the new
// tables — a leading comment block a blank line clear of the first header, a
// key-value written outside any table, and the named tables already there.
func topInsertAt(data []byte, keys [][]string) (int, error) {
	p := &unstable.Parser{KeepComments: true}
	p.Reset(data)
	// Current run of contiguous full-line comments: a run ending on the line
	// right above a header annotates that header and moves with it.
	runStart, runEnd := -1, -1
	for p.NextExpression() {
		e := p.Expression()
		switch e.Kind {
		case unstable.Comment:
			start := lineStart(data, int(e.Raw.Offset))
			if runEnd != start {
				runStart = start
			}
			runEnd = lineEnd(data, int(e.Raw.Offset+e.Raw.Length))
			continue
		case unstable.Table, unstable.ArrayTable:
			path, line := headerAt(data, e)
			if !named(keys, path) {
				if runEnd == line {
					return runStart, nil
				}
				return line, nil
			}
		}
		runStart, runEnd = -1, -1
	}
	return len(data), p.Error()
}

// named reports whether one of keys is path or a parent of it, so that a
// table already at the top of the document, and its sub-tables, keep their
// place instead of being pushed down by the new ones.
func named(keys [][]string, path []string) bool {
	for _, key := range keys {
		if len(key) <= len(path) && slices.Equal(key, path[:len(key)]) {
			return true
		}
	}
	return false
}

// headerAt returns a table header's key path and the start of the line it is
// on.
func headerAt(data []byte, e *unstable.Node) (path []string, line int) {
	it := e.Key()
	for it.Next() {
		if len(path) == 0 {
			// Only whitespace can come between the first key part and the
			// opening bracket, so it locates the header's line.
			line = lineStart(data, int(it.Node().Raw.Offset))
		}
		path = append(path, string(it.Node().Data))
	}
	return path, line
}

// headerKey renders a table header path, quoting the parts that need it the
// way the TOML encoder does.
func headerKey(key []string) (string, error) {
	parts := make([]string, len(key))
	for i, part := range key {
		line, err := toml.Marshal(map[string]any{part: 0})
		if err != nil {
			return "", err
		}
		parts[i] = strings.TrimSuffix(string(line), " = 0\n")
	}
	return strings.Join(parts, "."), nil
}

// eol returns the line ending to use for inserted lines: CRLF if the document
// already uses it, LF otherwise.
func eol(data []byte) []byte {
	if bytes.Contains(data, []byte("\r\n")) {
		return []byte("\r\n")
	}
	return []byte("\n")
}

func lineStart(data []byte, off int) int {
	return bytes.LastIndexByte(data[:off], '\n') + 1
}

func lineEnd(data []byte, off int) int {
	if i := bytes.IndexByte(data[off:], '\n'); i >= 0 {
		return off + i + 1
	}
	return len(data)
}

// blankLineBefore reports whether the line ending at pos (excluded) is empty.
// Only meaningful when pos is a line boundary.
func blankLineBefore(data []byte, pos int) bool {
	i := pos - 1 // data[i] == '\n'
	if i > 0 && data[i-1] == '\r' {
		i--
	}
	return i == 0 || data[i-1] == '\n'
}
