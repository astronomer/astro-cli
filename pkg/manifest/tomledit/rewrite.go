package tomledit

import (
	"fmt"

	toml "github.com/pelletier/go-toml/v2"
)

// rewrite is the fallback implementation: it decodes the document to plain
// data and re-marshals it on Bytes, so comments, key order, and hand
// formatting are lost.
type rewrite struct {
	root map[string]any
}

// NewRewrite returns an Editor that rewrites the whole document, keeping
// only its data.
func NewRewrite(src []byte) (Editor, error) {
	root := map[string]any{}
	if err := toml.Unmarshal(src, &root); err != nil {
		return nil, &ParseError{Err: err}
	}
	return &rewrite{root: root}, nil
}

func (r *rewrite) Get(key []string) (any, bool) {
	// Decode through a marshal round trip so values Set in Go types come
	// back as their TOML decoding (int64, not int), like the surgical
	// editor, and so callers never hold a reference into live state.
	b, err := toml.Marshal(r.root)
	if err != nil {
		return nil, false
	}
	root := map[string]any{}
	if err := toml.Unmarshal(b, &root); err != nil {
		return nil, false
	}
	var v any = root
	for _, k := range key {
		switch c := v.(type) {
		case map[string]any:
			child, ok := c[k]
			if !ok {
				return nil, false
			}
			v = child
		case []any:
			idx, err := arrayIndex(k, key)
			if err != nil || idx >= len(c) {
				return nil, false
			}
			v = c[idx]
		default:
			return nil, false
		}
	}
	return v, true
}

func (r *rewrite) Set(key []string, value any) error {
	if len(key) == 0 {
		return &KeyError{Key: key, Reason: "empty key"}
	}
	// Probe the value alone so a bad one fails here, like the surgical
	// editor, not later in Bytes.
	if _, err := toml.Marshal(map[string]any{"probe": value}); err != nil {
		return &KeyError{Key: key, Err: err}
	}
	c, err := setIn(r.root, key, key, value)
	if err != nil {
		return err
	}
	r.root = c.(map[string]any)
	return nil
}

func (r *rewrite) EnsureTablesAtTop(keys [][]string) error {
	// This implementation re-marshals the whole document, so it keeps no
	// order to place the tables in: they only have to exist.
	for _, key := range keys {
		if len(key) == 0 {
			return &KeyError{Key: key, Reason: "empty key"}
		}
		if _, ok := r.Get(key); ok {
			continue
		}
		if _, err := setIn(r.root, key, key, map[string]any{}); err != nil {
			return err
		}
	}
	return nil
}

func (r *rewrite) Delete(key []string) bool {
	if len(key) == 0 {
		return false
	}
	c, present := deleteIn(r.root, key)
	r.root = c.(map[string]any)
	return present
}

func (r *rewrite) Bytes() ([]byte, error) {
	return toml.Marshal(r.root)
}

// tableLike reports values Set must not overwrite: tables and arrays of
// tables, mirroring the surgical editor's rule that replacing them
// wholesale takes an explicit Delete first.
func tableLike(v any) bool {
	switch c := v.(type) {
	case map[string]any:
		return true
	case []any:
		if len(c) == 0 {
			return false
		}
		_, ok := c[0].(map[string]any)
		return ok
	}
	return false
}

// setIn writes value at key inside container c and returns the container,
// re-allocated when an array append grew it. full is the whole path, for
// errors.
func setIn(c any, key, full []string, value any) (any, error) {
	switch container := c.(type) {
	case map[string]any:
		k := key[0]
		if len(key) == 1 {
			if existing, ok := container[k]; ok && tableLike(existing) {
				return nil, &KeyError{Key: full, Reason: "already a table; delete it first"}
			}
			container[k] = value
			return container, nil
		}
		child, ok := container[k]
		if !ok {
			child = map[string]any{}
		}
		newChild, err := setIn(child, key[1:], full, value)
		if err != nil {
			return nil, err
		}
		container[k] = newChild
		return container, nil
	case []any:
		idx, err := arrayIndex(key[0], full)
		if err != nil {
			return nil, err
		}
		switch {
		case idx < len(container):
			if len(key) == 1 {
				if tableLike(container[idx]) {
					return nil, &KeyError{Key: full, Reason: "already a table; delete it first"}
				}
				container[idx] = value
				return container, nil
			}
			newChild, err := setIn(container[idx], key[1:], full, value)
			if err != nil {
				return nil, err
			}
			container[idx] = newChild
			return container, nil
		case idx == len(container):
			if len(key) == 1 {
				return append(container, value), nil
			}
			newChild, err := setIn(map[string]any{}, key[1:], full, value)
			if err != nil {
				return nil, err
			}
			return append(container, newChild), nil
		default:
			return nil, &KeyError{Key: full, Reason: fmt.Sprintf("index %d out of range for array of %d", idx, len(container))}
		}
	default:
		return nil, &KeyError{Key: full, Reason: fmt.Sprintf("%q is not a table or array", key[0])}
	}
}

// deleteIn removes the value at key inside container c, returning the
// container and whether the value was present.
func deleteIn(c any, key []string) (any, bool) {
	switch container := c.(type) {
	case map[string]any:
		k := key[0]
		if len(key) == 1 {
			_, ok := container[k]
			delete(container, k)
			return container, ok
		}
		child, ok := container[k]
		if !ok {
			return container, false
		}
		newChild, present := deleteIn(child, key[1:])
		// A table left empty by the delete goes too, like the surgical
		// editor removing the last section under an implicit parent.
		if m, ok := newChild.(map[string]any); ok && present && len(m) == 0 {
			delete(container, k)
			return container, true
		}
		container[k] = newChild
		return container, present
	case []any:
		idx, err := arrayIndex(key[0], key)
		if err != nil || idx >= len(container) {
			return container, false
		}
		if len(key) == 1 {
			return append(container[:idx], container[idx+1:]...), true
		}
		newChild, present := deleteIn(container[idx], key[1:])
		container[idx] = newChild
		return container, present
	default:
		return container, false
	}
}

// arrayIndex parses a path element used to step into an array, mirroring
// the surgical editor's rule exactly: a non-empty string of decimal
// digits. Signs are rejected; leading zeros are digits and allowed.
func arrayIndex(s string, full []string) (int, error) {
	bad := func() (int, error) {
		return 0, &KeyError{Key: full, Reason: fmt.Sprintf("%q is not an array index", s)}
	}
	if s == "" || len(s) > 18 {
		return bad()
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return bad()
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, nil
}
