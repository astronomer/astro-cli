package cliout

// The text half of output: how a renderer writes lines without checking every
// one, and the table the CLI's lists print (pkg/texttable's). Both are the
// standard library (bufio, text/tabwriter); there is no table package to learn.

import (
	"bufio"
	"io"

	"github.com/astronomer/astro-cli/pkg/texttable"
)

// WriteText runs render over a buffered writer on w and returns the one error
// that matters: whether the text reached w.
//
// A bufio.Writer's error is sticky. Once a write to w fails, every later write
// does nothing and the failure is kept until Flush returns it. So render writes
// its lines with fmt.Fprintf and fmt.Fprintln and checks none of them, and the
// check happens once, here, instead of after every line:
//
//	return cliout.WriteText(w, func(b *bufio.Writer) {
//		fmt.Fprintf(b, "[%s]\n", name)
//		for _, o := range options {
//			fmt.Fprintln(b, o.Key+" = "+o.Value)
//		}
//	})
//
// render returns nothing because nothing it does can fail: a renderer formats
// values it was handed. Work that can fail happens before the render.
func WriteText(w io.Writer, render func(b *bufio.Writer)) error {
	b := bufio.NewWriter(w)
	render(b)
	return b.Flush()
}

// Text is WriteText shaped as Emit's text renderer:
//
//	return r.Emit(result, cliout.Text(func(b *bufio.Writer) { ... }))
func Text(render func(b *bufio.Writer)) func(io.Writer) error {
	return func(w io.Writer) error { return WriteText(w, render) }
}

// Table is a list printed as aligned columns under a header row, the layout
// the CLI's tables have always had. It is texttable.Table, which lives in
// pkg/ so a picker below cmd/ (pkg/picker) draws its rows the same way.
type Table = texttable.Table
