// Package tomledit edits a TOML document by key path.
//
// Two implementations back the Editor interface. NewSurgical rewrites only
// the bytes an edit touches, preserving comments, key order, and formatting
// everywhere else; it rides go-toml's unstable/edit API, which is unreleased
// and pinned by pseudo-version. NewRewrite decodes the whole document and
// re-marshals it, losing comments and hand formatting. It exists as the
// fallback: if the unstable API churns and cannot be kept working, callers
// switch constructors and ship, rather than block a release.
//
// Both implementations agree on what an edit means; they differ only in how
// much of the original layout survives.
package tomledit

import (
	"errors"
	"fmt"
	"strings"
)

// Editor applies keyed edits to a TOML document.
type Editor interface {
	// Get returns the decoded value at key and whether it exists. Tables
	// decode as map[string]any, arrays as []any, scalars per the usual
	// TOML decoding. It is the read half of read-modify-write: check
	// existence before Set, or an array's length before appending.
	Get(key []string) (any, bool)
	// Set writes value at key, one path element per key part, creating
	// missing intermediate tables. A path element stepping into an array is
	// a 0-based decimal index; an index equal to the array's length appends.
	// Setting over an existing table (or array of tables) is an error:
	// delete it first to replace it wholesale. The surgical editor writes a
	// []any of scalars that is the value of a key-value line one element per
	// line, as uv does, including one an append lands in.
	Set(key []string, value any) error
	// EnsureTablesAtTop creates each table of keys the document does not
	// have yet, empty and in the order given, at the top of the document:
	// above the first table the document has that keys does not name. A
	// table already there keeps its place, and so does a leading comment
	// block. Later Sets fill the tables where they now are.
	EnsureTablesAtTop(keys [][]string) error
	// AppendArrayTable adds an empty element to the array of tables at key,
	// creating the array when the document has none, and returns the new
	// element's index for Set to fill: Set can extend an array of tables but
	// cannot start one.
	AppendArrayTable(key []string) (int, error)
	// Delete removes the value at key — a scalar, a whole table, or an
	// array element — and reports whether it was present.
	Delete(key []string) bool
	// Bytes returns the edited document.
	Bytes() ([]byte, error)
}

// Comment is a comment line inside an array, as GetLines reads it and Set
// writes it: the text after the '#' and the one space that follows it. It
// is never a value, and Get leaves it out.
type Comment string

// LineGetter is an Editor that reads an array with its comments. Both editors
// here are one; an Editor from elsewhere need not be, which is why it is not
// part of Editor.
type LineGetter interface {
	// GetLines is Get for the array at key, with the comments between its
	// elements in place as Comment values, so a Set of what it returns keeps
	// them. It reports false when the value at key is not an array. An
	// editor that keeps no comments returns the elements alone.
	GetLines(key []string) ([]any, bool)
}

// GetLines reads the array at key in e with its comments when e is a
// LineGetter, and its elements alone, as Get decodes them, when it is not.
// It reports false when the value at key is not an array.
func GetLines(e Editor, key []string) ([]any, bool) {
	if lines, ok := e.(LineGetter); ok {
		return lines.GetLines(key)
	}
	v, ok := e.Get(key)
	if !ok {
		return nil, false
	}
	list, isArray := v.([]any)
	return list, isArray
}

// LineSetter is an Editor that writes an array line for line, comments
// included. The surgical editor is one; an Editor from elsewhere need not be.
type LineSetter interface {
	// SetLines writes list at key as Set does, but compares it with the
	// array there comments and all, so one whose values are unchanged and
	// whose comments are not is rewritten.
	SetLines(key []string, list []any) error
}

// SetLines writes list at key in e, line for line when e is a LineSetter,
// and by Set, whose values alone it compares, when it is not.
func SetLines(e Editor, key []string, list []any) error {
	if lines, ok := e.(LineSetter); ok {
		return lines.SetLines(key, list)
	}
	return e.Set(key, list)
}

// ErrCommentsUnsupported refuses a Set of an array holding a Comment where the
// array cannot carry one: inside an inline table, where TOML allows no line
// break. Nothing is written.
var ErrCommentsUnsupported = errors.New("this array is inside an inline value, which holds no comment")

// Values is list without its comments.
func Values(list []any) []any {
	out := make([]any, 0, len(list))
	for _, v := range list {
		if _, isComment := v.(Comment); !isComment {
			out = append(out, v)
		}
	}
	return out
}

// hasComment reports whether value is an array holding a Comment.
func hasComment(value any) bool {
	list, ok := value.([]any)
	if !ok {
		return false
	}
	for _, v := range list {
		if _, isComment := v.(Comment); isComment {
			return true
		}
	}
	return false
}

// ParseError reports input that does not decode as TOML.
type ParseError struct {
	Err error
}

func (e *ParseError) Error() string { return fmt.Sprintf("parse toml: %v", e.Err) }

func (e *ParseError) Unwrap() error { return e.Err }

// KeyError reports an edit that could not be applied at its key.
type KeyError struct {
	Key    []string
	Reason string
	Err    error // nil when Reason says it all
}

func (e *KeyError) Error() string {
	reason := e.Reason
	if reason == "" && e.Err != nil {
		reason = e.Err.Error()
	}
	return fmt.Sprintf("edit %s: %s", strings.Join(e.Key, "."), reason)
}

func (e *KeyError) Unwrap() error { return e.Err }
