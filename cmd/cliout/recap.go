package cliout

import (
	"io"
	"sync"
)

// Recap shows a run's warnings and notes as they are found, and repeats them
// after the run's progress output — an image build's, most often — so they
// are not scrolled away above it. Shown when found, a warning reaches a user
// who interrupts the run and the log of a CI job that is killed; repeated at
// the end, it is the last thing read before the result.
//
// Only what progress output has followed is repeated: a warning found after
// the last of it is already just above the result, so End prints it once. A
// warning the command adds once the work is over (Later) prints once, at the
// end. The command calls End just before its result, or before the error a
// failed run returns.
//
// Safe for concurrent use: a build streams its output from goroutines of its
// own.
type Recap struct {
	mu     sync.Mutex
	show   func(note bool, msg string)
	repeat bool
	items  []recapItem
}

type recapItem struct {
	note bool
	msg  string
	// shown is whether it was printed when found; buried, whether progress
	// output has followed it since.
	shown, buried bool
}

// NewRecap prints warnings led by Warning: and notes led by Note: to w. With
// repeat unset (json, where warnings stream once, in order), nothing is
// repeated.
func NewRecap(w io.Writer, repeat bool) *Recap {
	return NewRecapFunc(func(note bool, msg string) {
		if note {
			Note(w, msg)
		} else {
			Warn(w, msg)
		}
	}, repeat)
}

// NewRecapFunc is NewRecap with show printing each one, for a command whose
// warnings are not lines (json events).
func NewRecapFunc(show func(note bool, msg string), repeat bool) *Recap {
	return &Recap{show: show, repeat: repeat}
}

// Warn shows msg now, to repeat at the end.
func (r *Recap) Warn(msg string) { r.found(false, msg) }

// Note shows msg now, to repeat at the end.
func (r *Recap) Note(msg string) { r.found(true, msg) }

func (r *Recap) found(note bool, msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.show(note, msg)
	r.items = append(r.items, recapItem{note: note, msg: msg, shown: true})
}

// Later holds a warning found once the work is over, to show once, at the end.
func (r *Recap) Later(msg string) { r.later(false, msg) }

// LaterNote is Later for a note.
func (r *Recap) LaterNote(msg string) { r.later(true, msg) }

func (r *Recap) later(note bool, msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, recapItem{note: note, msg: msg})
}

// Progress records that progress output was printed, after every warning and
// note shown so far.
func (r *Recap) Progress() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.items {
		r.items[i].buried = true
	}
}

// End prints, in the order they came, the warnings and notes progress output
// has followed since they were shown, and the ones held for the end, and
// empties the recap.
func (r *Recap) End() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, it := range r.items {
		if !it.shown || (r.repeat && it.buried) {
			r.show(it.note, it.msg)
		}
	}
	r.items = nil
}
