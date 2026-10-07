package input

import (
	"bufio"
	"io"
	"os"
	"sync"
)

// shared is the one buffered reader the process's questions read their
// answers through, and the file it buffers.
//
// A bufio.Reader reads ahead: given a pipe, its first read takes everything
// that is there, not just the line it returns. So two questions that each
// wrapped stdin in a reader of their own lost answers between them: a picker
// took "2\ny\n" from a pipe, used the "2", and the confirmation after it read
// a stdin with nothing left, which is a no. A terminal hands over one line
// per read, so only piped answers ever showed it. Every question reading the
// same file reads through the same reader, and so sees what the one before
// it left buffered.
var shared struct {
	sync.Mutex
	src *os.File
	r   *bufio.Reader
}

// Reader returns the buffered reader questions read in through. When in is a
// file (stdin is one everywhere: os.Stdin, cmd.InOrStdin(), a harness's
// os.Pipe) it is the same reader for as long as in is the same file, so an
// answer one question read ahead is there for the next; another file (a test
// replacing os.Stdin with a pipe of its own, say) gets a reader of its own,
// and nothing read ahead from the old one carries over. in that is already a
// *bufio.Reader is returned as it is, and any other source is read through a
// reader of its own, as it always was.
func Reader(in io.Reader) *bufio.Reader {
	switch src := in.(type) {
	case *bufio.Reader:
		return src
	case *os.File:
		if src == nil {
			break
		}
		shared.Lock()
		defer shared.Unlock()
		if shared.r == nil || shared.src != src {
			shared.src, shared.r = src, bufio.NewReader(src)
		}
		return shared.r
	}
	return bufio.NewReader(in)
}

// stdin is the reader on os.Stdin as it is now.
func stdin() *bufio.Reader { return Reader(os.Stdin) }
