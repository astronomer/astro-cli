package input

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

// ErrInterrupted is HiddenValue's answer to Ctrl-C. The terminal is raw while
// it reads, so the keypress arrives as a byte rather than a signal, and the
// read has to say what the signal would have.
var ErrInterrupted = errors.New("interrupted")

const (
	keyCtrlC     = 0x03
	keyCtrlD     = 0x04
	keyBackspace = 0x08
	keyCtrlU     = 0x15
	keyEscape    = 0x1b
	keyDelete    = 0x7f
)

// HiddenValue reads a value from the terminal f without echoing it, for a
// secret that may be long or span lines — a connection's JSON, most often.
//
// term.ReadPassword cannot take one. It reads in the terminal's line mode,
// which ends the value at the first newline, so pasted pretty-printed JSON
// arrived as a lone "{". And macOS caps a line in that mode at 1024 bytes and
// drops everything past it, Enter included, so a pasted service-account key
// left the prompt waiting for a newline that could never arrive. Raw mode has
// neither limit, at the cost of doing the line editing here.
//
// Enter ends the value, except inside a JSON object or array that is still
// open: there it is a newline in the value, so a paste ends where the JSON
// does. Ctrl-D ends the value anywhere, for JSON that never closes.
func HiddenValue(f *os.File) (string, error) {
	fd := int(f.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return "", err
	}
	defer term.Restore(fd, state) //nolint:errcheck // nothing to do if the terminal cannot be restored
	return readHidden(f)
}

// readHidden is HiddenValue's line editing, over the raw bytes a terminal sends.
func readHidden(r io.Reader) (string, error) {
	br := bufio.NewReader(r)
	var buf []byte
	var prev byte
	for {
		b, err := br.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) && len(buf) > 0 {
				return string(buf), nil
			}
			return "", err
		}
		crlf := prev == '\r' && b == '\n'
		prev = b
		switch b {
		case keyCtrlC:
			return "", ErrInterrupted
		case keyCtrlD:
			if len(buf) == 0 {
				return "", io.EOF
			}
			return string(buf), nil
		case '\r', '\n':
			if crlf {
				continue // one line ending, already handled at the \r
			}
			if !openJSON(string(buf)) {
				return string(buf), nil
			}
			buf = append(buf, '\n')
		case keyBackspace, keyDelete:
			if len(buf) > 0 {
				_, size := utf8.DecodeLastRune(buf)
				buf = buf[:len(buf)-size]
			}
		case keyCtrlU:
			buf = buf[:0]
		case keyEscape:
			skipEscape(br)
		default:
			if b >= 0x20 || b == '\t' {
				buf = append(buf, b)
			}
		}
	}
}

// skipEscape discards the rest of an escape sequence — an arrow key, say — so
// it does not land in the value. A CSI sequence ("ESC [") runs to its final
// byte, 0x40 through 0x7e; anything else is one more byte.
func skipEscape(br *bufio.Reader) {
	b, err := br.ReadByte()
	if err != nil || b != '[' {
		return
	}
	for {
		b, err := br.ReadByte()
		if err != nil || (b >= 0x40 && b <= 0x7e) {
			return
		}
	}
}

// openJSON reports whether s starts a JSON object or array that has not
// closed yet, counting brackets outside strings. It does not validate:
// whoever reads the value does that, with a better error than a prompt can.
func openJSON(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || (s[0] != '{' && s[0] != '[') {
		return false
	}
	depth, inString, escaped := 0, false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case escaped:
			escaped = false
		case inString && c == '\\':
			escaped = true
		case c == '"':
			inString = !inString
		case inString:
		case c == '{' || c == '[':
			depth++
		case c == '}' || c == ']':
			depth--
		}
	}
	return depth > 0
}
