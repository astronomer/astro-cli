package rt

import "testing"

// A subprocess writes whole lines, but nothing guarantees the write boundaries
// line up with them — so a chunk ending mid-line must be held until its newline
// arrives, and Flush must still emit a trailing line that never gets one.
//
// Moved here with LineWriter itself, from internal/localshared. Rewritten off
// testify on the way: pkg/localrt's go.mod has no dependencies, which is the
// property that makes it cheap for the desktop to import, and a line-splitting
// test is not worth spending it.
func TestLineWriterBuffersPartialWrites(t *testing.T) {
	t.Parallel()
	var got []string
	w := &LineWriter{Emit: func(s string) { got = append(got, s) }}

	for _, chunk := range []string{"first li", "ne\r\nsecond", " line\npart", "ial"} {
		if _, err := w.Write([]byte(chunk)); err != nil {
			t.Fatalf("Write(%q) = %v, want nil", chunk, err)
		}
	}
	if len(got) != 2 || got[0] != "first line" || got[1] != "second line" {
		t.Errorf("emitted %q, want [first line, second line] — a partial line must be buffered until its newline arrives", got)
	}
	w.Flush()
	if len(got) != 3 || got[2] != "partial" {
		t.Errorf("after Flush emitted %q, want the trailing newline-less line too", got)
	}
}
