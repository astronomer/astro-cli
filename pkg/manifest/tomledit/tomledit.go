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
	// Delete removes the value at key — a scalar, a whole table, or an
	// array element — and reports whether it was present.
	Delete(key []string) bool
	// Bytes returns the edited document.
	Bytes() ([]byte, error)
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
