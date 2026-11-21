package termtext

import "testing"

// Whole escape sequences go, in either form: the colors a log line carries,
// a title, a hyperlink, a clipboard write, a device control string.
func TestLinesDropsEscapes(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"plain":      {"no escapes", "no escapes"},
		"csi color":  {"\x1b[33mscheduler\x1b[0m | ok", "scheduler | ok"},
		"osc bel":    {"a\x1b]52;c;aGk=\x07b", "ab"},
		"osc st":     {"a\x1b]0;title\x1b\\b", "ab"},
		"osc link":   {"\x1b]8;;http://x\x07text\x1b]8;;\x07", "text"},
		"dcs string": {"a\x1bPpayload\x1b\\b", "ab"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := Lines(tc.in); got != tc.want {
				t.Errorf("Lines(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// What a terminal would act on goes too: a lone ESC or C1 control (the text
// after it stays, since without it the terminal acts on none of it), the C1
// form of an OSC or a device control string with its payload, any other
// control character (a carriage return would let a later part of a line
// overwrite an earlier one), DEL, the characters that reorder or hide text,
// and an OSC that never finished, with the rest of its line, so its payload
// does not print as text.
func TestLinesDropsControlCharacters(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"lone esc":             {"a\x1b\x01b", "ab"},
		"esc at end":           {"abc\x1b", "abc"},
		"lone esc before text": {"a\x1bb", "ab"},
		"esc then letters":     {"Delete \x1bprod?", "Delete prod?"},
		"reset":                {"a\x1bcb", "acb"},
		"save cursor":          {"a\x1b7b", "a7b"},
		"charset designation":  {"a\x1b(Bb", "a(Bb"},
		"c1 control":           {"a\u009bb", "ab"},
		"c1 csi":               {"a\u009b2Jb", "a2Jb"},
		"c1 osc":               {"a\u009d0;title\u009cb", "ab"},
		"c1 other":             {"a\u0085b\u0090x\u009cc", "abc"},
		"bell":                 {"a\x07b", "ab"},
		"backspace":            {"a\x08b", "ab"},
		"carriage return":      {"progress 10%\rprogress 100%", "progress 10%progress 100%"},
		"trailing cr":          {"line\r", "line"},
		"crlf":                 {"one\r\ntwo", "one\ntwo"},
		"del":                  {"a\x7fb", "ab"},
		"nul and form feed":    {"a\x00b\x0cc", "abc"},
		"bidi override":        {"run \u202eabc\u202c", "run abc"},
		"zero width space":     {"a\u200bb\ufeff", "ab"},
		"unterminated osc":     {"ok \x1b]0;evil title", "ok "},
		"unterminated osc esc": {"ok \x1b]0;evil title\x1b", "ok "},
		"esc then capital":     {"ok \x1bPhoenix", "ok Phoenix"},
		"unterminated csi":     {"ok \x1b[31", "ok "},
		"unterminated c1 osc":  {"ok \u009d0;title", "ok "},
		"finished then broken": {"\x1b[1mbold\x1b[0m done \x1b]8;;http://x", "bold done "},
		"unfinished ends line": {"a \x1b]0;title\nnext line", "a \nnext line"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := Lines(tc.in); got != tc.want {
				t.Errorf("Lines(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Text with nothing to remove comes back as it is: tabs and line breaks,
// accents, emoji (with the zero-width joiners they are built from), CJK, and
// characters whose UTF-8 merely starts like a C1 control's.
func TestLinesKeepsTextAsIs(t *testing.T) {
	for name, in := range map[string]string{
		"tab":          "a\tb",
		"lines":        "one\ntwo",
		"accents":      "caf\u00e9 r\u00e9sum\u00e9",
		"emoji":        "done \U0001F680 ok",
		"cjk":          "\u65e5\u672c\u8a9e\u306e\u30ed\u30b0",
		"zwj emoji":    "family \U0001F468\u200d\U0001F469\u200d\U0001F467",
		"c1 lookalike": "\u00c2ge \u00c2 \u00a0nbsp",
		"empty":        "",
	} {
		t.Run(name, func(t *testing.T) {
			if got := Lines(in); got != in {
				t.Errorf("Lines(%q) = %q, want it unchanged", in, got)
			}
		})
	}
}

// Strip removes the same, and its white space only when asked to keep it not.
func TestStripKeepsWhiteSpaceOnlyWhenAsked(t *testing.T) {
	in := "a\tb\nc\x1b]0;t\x07\u202e"
	if got := Strip(in, true); got != "a\tb\nc" {
		t.Errorf("Strip(keepSpace) = %q", got)
	}
	if got := Strip(in, false); got != "abc" {
		t.Errorf("Strip = %q", got)
	}
}

// The usual line, with nothing to remove, costs no allocation.
func TestPlainTextDoesNotAllocate(t *testing.T) {
	for _, line := range []string{
		"[2026-01-01] INFO scheduler | heartbeat\tok",
		"caf\u00e9 \u65e5\u672c\u8a9e \U0001F680",
	} {
		if n := testing.AllocsPerRun(100, func() { _ = Lines(line); _ = Strip(line, true) }); n != 0 {
			t.Errorf("%q: %v allocations, want 0", line, n)
		}
	}
}
