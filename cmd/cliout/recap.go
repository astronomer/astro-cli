package cliout

import (
	"io"
	"sync"
)

// Recap collects a run's warnings and notes and shows each once. In text it
// holds them until Flush, which the command calls just before its result, or
// before the error a failed run returns, so they print after the run's
// progress output — an image build's, most often — where a long build cannot
// scroll them away, and are the last thing read before the result. A command
// that asks a question calls Flush before asking too, so what it found so far
// is read before the answer is given; those print there and not again.
//
// Not holding (json, where warnings stream in order), each shows as it is
// found.
//
// Safe for concurrent use: a build streams its output from goroutines of its
// own.
type Recap struct {
	mu    sync.Mutex
	show  func(note bool, msg string)
	hold  bool
	items []recapItem
}

type recapItem struct {
	note bool
	msg  string
}

// NewRecap prints warnings led by Warning: and notes led by Note: to w. With
// hold unset, each prints as it is found.
func NewRecap(w io.Writer, hold bool) *Recap {
	return NewRecapFunc(func(note bool, msg string) {
		if note {
			Note(w, msg)
		} else {
			Warn(w, msg)
		}
	}, hold)
}

// NewRecapFunc is NewRecap with show printing each one, for a command whose
// warnings are not lines (json events).
func NewRecapFunc(show func(note bool, msg string), hold bool) *Recap {
	return &Recap{show: show, hold: hold}
}

// Warn shows msg at the next Flush, or now when the recap does not hold.
func (r *Recap) Warn(msg string) { r.found(false, msg) }

// Note is Warn for a note.
func (r *Recap) Note(msg string) { r.found(true, msg) }

func (r *Recap) found(note bool, msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.hold {
		r.show(note, msg)
		return
	}
	r.items = append(r.items, recapItem{note: note, msg: msg})
}

// Flush prints the held warnings and notes in the order they came, and
// empties the recap.
func (r *Recap) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, it := range r.items {
		r.show(it.note, it.msg)
	}
	r.items = nil
}
