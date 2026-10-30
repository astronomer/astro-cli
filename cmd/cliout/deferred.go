package cliout

import (
	"io"

	"github.com/astronomer/astro-cli/pkg/input"
)

// Deferred holds the warnings and notes a text-mode run finds as it goes, so
// they print together after the run's progress output — an image build's,
// most often — rather than scrolled away above it. The command flushes it
// just before its result, or before the error a failed run returns, so a
// failure still shows what was found on the way to it.
//
// Text mode only: under json the command's warnings stream in order, so a
// Deferred made not to hold prints each one as it is given.
type Deferred struct {
	w     io.Writer
	hold  bool
	items []deferredItem
}

type deferredItem struct {
	note bool
	msg  string
}

// NewDeferred prints warnings and notes to w: held until Flush when hold is
// set, and as they are given otherwise.
func NewDeferred(w io.Writer, hold bool) *Deferred {
	return &Deferred{w: w, hold: hold}
}

// Warn prints msg led by Warning:, or holds it to.
func (d *Deferred) Warn(msg string) {
	d.add(deferredItem{msg: msg})
}

// Note prints msg led by Note:, or holds it to.
func (d *Deferred) Note(msg string) {
	d.add(deferredItem{note: true, msg: msg})
}

func (d *Deferred) add(it deferredItem) {
	if d.hold {
		d.items = append(d.items, it)
		return
	}
	d.print(it)
}

// FlushBeforeQuestions has every question the run asks (input.MayAsk, which
// the prompts and pickers all pass through) flush what is held first, so a
// warning found before a question is read before it is answered; what is
// found after the question is held for the end as before. It returns the
// function that removes the hook, for the command to defer.
func (d *Deferred) FlushBeforeQuestions() (restore func()) {
	return input.BeforeAsk(d.Flush)
}

// Flush prints what is held, in the order it came, and empties it.
func (d *Deferred) Flush() {
	for _, it := range d.items {
		d.print(it)
	}
	d.items = nil
}

func (d *Deferred) print(it deferredItem) {
	if it.note {
		Note(d.w, it.msg)
	} else {
		Warn(d.w, it.msg)
	}
}
