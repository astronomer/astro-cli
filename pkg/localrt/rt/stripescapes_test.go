package rt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripEscapesDropsCSIAndOSC(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"plain":      {"no escapes", "no escapes"},
		"csi color":  {"\x1b[33mscheduler\x1b[0m | ok", "scheduler | ok"},
		"osc bel":    {"a\x1b]52;c;aGk=\x07b", "ab"},
		"osc st":     {"a\x1b]0;title\x1b\\b", "ab"},
		"osc link":   {"\x1b]8;;http://x\x07text\x1b]8;;\x07", "text"},
		"dcs string": {"a\x1bPpayload\x1b\\b", "ab"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, StripEscapes(tc.in))
		})
	}
}

func TestStripEscapesDropsControlCharacters(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"lone esc":             {"a\x1b\x01b", "ab"},
		"esc at end":           {"abc\x1b", "abc"},
		"reset":                {"a\x1bcb", "ab"},
		"save cursor":          {"a\x1b7b", "ab"},
		"charset designation":  {"a\x1b(Bb", "ab"},
		"c1 csi":               {"a\u009b2Jb", "ab"},
		"c1 osc":               {"a\u009d0;title\u009cb", "ab"},
		"c1 other":             {"a\u0085b\u0090x\u009cc", "abc"},
		"bell":                 {"a\x07b", "ab"},
		"backspace":            {"a\x08b", "ab"},
		"carriage return":      {"progress 10%\rprogress 100%", "progress 10%progress 100%"},
		"trailing cr":          {"line\r", "line"},
		"del":                  {"a\x7fb", "ab"},
		"nul and form feed":    {"a\x00b\x0cc", "abc"},
		"bidi override":        {"run \u202eabc\u202c", "run abc"},
		"zero width space":     {"a\u200bb\ufeff", "ab"},
		"unterminated osc":     {"ok \x1b]0;evil title", "ok "},
		"unterminated osc esc": {"ok \x1b]0;evil title\x1b", "ok "},
		"unterminated dcs":     {"ok \x1bPpayload", "ok "},
		"unterminated csi":     {"ok \x1b[31", "ok "},
		"unterminated c1 osc":  {"ok \u009d0;title", "ok "},
		"finished then broken": {"\x1b[1mbold\x1b[0m done \x1b]8;;http://x", "bold done "},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, StripEscapes(tc.in))
		})
	}
}

func TestStripEscapesKeepsTextAsIs(t *testing.T) {
	for name, in := range map[string]string{
		"tab":          "a\tb",
		"accents":      "café résumé",
		"emoji":        "done \U0001F680 ok",
		"cjk":          "日本語のログ",
		"zwj emoji":    "family \U0001F468\u200d\U0001F469\u200d\U0001F467",
		"c1 lookalike": "Âge Â \u00a0nbsp",
		"empty":        "",
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, in, StripEscapes(in))
		})
	}
}

func TestStripEscapesPlainLineDoesNotAllocate(t *testing.T) {
	line := "[2026-01-01] INFO scheduler | heartbeat\tok"
	assert.Zero(t, testing.AllocsPerRun(100, func() { _ = StripEscapes(line) }))
	utf8 := "café 日本語 \U0001F680"
	assert.Zero(t, testing.AllocsPerRun(100, func() { _ = StripEscapes(utf8) }))
}
