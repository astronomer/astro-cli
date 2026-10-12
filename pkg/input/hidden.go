package input

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"unicode/utf8"

	"golang.org/x/term"
)

// ErrInterrupted is HiddenValue's answer to Ctrl-C. The terminal is raw while
// it reads, so the keypress arrives as a byte rather than a signal, and the
// read has to say what the signal would have; cliout.ExitCode turns it into
// the 130 an interrupt exits with.
var ErrInterrupted = errors.New("interrupted")

const (
	keyCtrlC     = 0x03
	keyCtrlD     = 0x04
	keyBackspace = 0x08
	keyCtrlU     = 0x15
	keyEscape    = 0x1b
	keyDelete    = 0x7f

	// Bracketed paste: a terminal asked to (pasteOn) wraps everything pasted
	// in pasteStart and pasteEnd, CSI sequences whose parameter is 200 and 201.
	pasteOn    = "\x1b[?2004h"
	pasteOff   = "\x1b[?2004l"
	pasteStart = "200~"
	pasteEnd   = "201~"

	// The shell's exit statuses for a process ended by these signals, 128+n.
	exitSIGHUP  = 129
	exitSIGTERM = 143
)

// HiddenValue reads a value from the terminal in without echoing it, for a
// secret that may be long or span lines — a connection's JSON, a key file.
//
// term.ReadPassword cannot take one. It reads in the terminal's line mode,
// which ends the value at the first newline, so pasted pretty-printed JSON
// arrived as a lone "{" and the rest of the paste ran as shell commands. And
// macOS caps a line in that mode at 1024 bytes and drops everything past it,
// Enter included, so a pasted service-account key left the prompt waiting for
// a newline that could never arrive. Raw mode has neither limit, at the cost
// of doing the line editing here.
//
// A paste is told from typing by bracketed paste, which out — the terminal the
// prompt is written to, or nil — is asked to turn on: a newline inside a paste
// is part of the value, and a typed Enter ends it. A paste that ends in a
// newline ends the value too, as pasting a line always has. A terminal without
// bracketed paste sends a paste as typing, so there a newline ends the value
// as it did before.
func HiddenValue(in, out *os.File) (string, error) {
	fd := int(in.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return "", err
	}
	if out != nil && !term.IsTerminal(int(out.Fd())) {
		out = nil
	}
	restore := func() {
		if out != nil {
			fmt.Fprint(out, pasteOff) //nolint:errcheck // best effort: the terminal is going back either way
		}
		term.Restore(fd, state) //nolint:errcheck // nothing to do if the terminal cannot be restored
	}
	defer restore()

	// A raw terminal outlives the process unless it is put back, and a defer
	// does not run when a signal ends the process. Ctrl-C arrives as a byte
	// here, but a SIGTERM or a closed window does not, and either would
	// otherwise leave the shell with no echo until the user runs reset.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case sig := <-sigs:
			restore()
			code := exitSIGTERM
			if sig == syscall.SIGHUP {
				code = exitSIGHUP
			}
			os.Exit(code)
		case <-done:
		}
	}()

	if out != nil {
		fmt.Fprint(out, pasteOn) //nolint:errcheck // without it a paste reads as typing, which still works
	}
	return readHidden(in)
}

// readHidden is HiddenValue's line editing, over the raw bytes a terminal
// sends. It reads one byte at a time: a buffered read would take bytes past
// the end of the value off the terminal and lose them.
func readHidden(r io.Reader) (string, error) {
	br := &byteReader{r: r}
	var buf []byte
	pasting, prevCR := false, false
	for {
		b, err := br.next()
		if err != nil {
			if errors.Is(err, io.EOF) && len(buf) > 0 {
				return finish(buf), nil
			}
			return "", err
		}
		if b == keyEscape {
			switch readEscape(br) {
			case pasteStart:
				pasting = true
			case pasteEnd:
				pasting = false
				if len(buf) > 0 && buf[len(buf)-1] == '\n' {
					return finish(buf), nil
				}
			}
			continue
		}
		if pasting {
			buf = pasted(buf, b, prevCR)
			prevCR = b == '\r'
			continue
		}
		var done bool
		if buf, done, err = typed(buf, b); done || err != nil {
			return finish(buf), err
		}
	}
}

// typed applies one typed key to buf, and reports whether it ends the value.
func typed(buf []byte, b byte) (next []byte, done bool, err error) {
	switch b {
	case keyCtrlC:
		return nil, true, ErrInterrupted
	case keyCtrlD:
		if len(buf) == 0 {
			return nil, true, io.EOF
		}
		return buf, true, nil
	case '\r', '\n':
		return buf, true, nil
	case keyBackspace, keyDelete:
		if len(buf) > 0 {
			_, size := utf8.DecodeLastRune(buf)
			buf = buf[:len(buf)-size]
		}
	case keyCtrlU:
		buf = buf[:0]
	default:
		if b >= 0x20 || b == '\t' {
			buf = append(buf, b)
		}
	}
	return buf, false, nil
}

// pasted adds one byte of a paste to buf. Terminals send a pasted line break
// as \r, \n or \r\n; each becomes one \n, so a \n right after a \r (prevCR)
// adds nothing. Other control bytes are dropped.
func pasted(buf []byte, b byte, prevCR bool) []byte {
	switch {
	case b == '\r', b == '\n' && !prevCR:
		return append(buf, '\n')
	case b >= 0x20 || b == '\t':
		return append(buf, b)
	}
	return buf
}

// finish is the value buf holds, without the line break a paste ends on.
func finish(buf []byte) string {
	return strings.TrimRight(string(buf), "\r\n")
}

// readEscape consumes the rest of an escape sequence and returns a CSI
// sequence's parameters and final byte ("200~"), or "" for anything else.
// A CSI sequence ("ESC [") runs to a final byte, 0x40 through 0x7e; an SS3
// one ("ESC O", function keys and application-mode arrows) is one more byte;
// any other byte after ESC means the ESC was a bare Esc key, which is dropped,
// and the byte goes back to be read as itself.
func readEscape(br *byteReader) string {
	b, err := br.next()
	if err != nil {
		return ""
	}
	switch b {
	case '[':
		var seq []byte
		for {
			c, err := br.next()
			if err != nil {
				return ""
			}
			seq = append(seq, c)
			if c >= 0x40 && c <= 0x7e {
				return string(seq)
			}
		}
	case 'O':
		br.next() //nolint:errcheck // an error here surfaces on the next read
	default:
		br.unread(b)
	}
	return ""
}

// byteReader reads one byte per call, so nothing past the value is taken off
// the terminal, and can put one byte back.
type byteReader struct {
	r       io.Reader
	one     [1]byte
	pending []byte
}

func (br *byteReader) unread(b byte) { br.pending = append(br.pending, b) }

func (br *byteReader) next() (byte, error) {
	if n := len(br.pending); n > 0 {
		b := br.pending[n-1]
		br.pending = br.pending[:n-1]
		return b, nil
	}
	for {
		n, err := br.r.Read(br.one[:])
		if n == 1 {
			return br.one[0], nil
		}
		if err != nil {
			return 0, err
		}
	}
}
