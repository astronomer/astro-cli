package tomledit

import (
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

func (s *surgical) Delete(key []string) bool {
	return s.doc.Delete(key)
}

func (s *surgical) Bytes() ([]byte, error) {
	return s.doc.Bytes(), nil
}
