package ansi

import (
	"bytes"
	"os"

	"github.com/logrusorgru/aurora"
)

func (s *Suite) TestStripBackticks() {
	cases := []struct{ name, in, want string }{
		{"a command", "Use `astro local start` instead.", "Use astro local start instead."},
		{"two spans", "`astro dev start` was removed. Use `astro local start`.", "astro dev start was removed. Use astro local start."},
		{"at the edges", "`x`", "x"},
		{"after punctuation", "(`astro use`; clear it with `astro use --unset`)", "(astro use; clear it with astro use --unset)"},
		{"a flag and a path", "add a line `.env` to it, or pass `--force`", "add a line .env to it, or pass --force"},
		{"a value with an equals sign", "mark a link `default = true` here", "mark a link default = true here"},
		{"no backticks", "plain text", "plain text"},
		{"empty", "", ""},
		{"unpaired", "a ` alone", "a ` alone"},
		{"unpaired at the end", "trailing `", "trailing `"},
		{"an empty pair", "an empty `` pair", "an empty `` pair"},
		{"a run of three", "```go\ncode\n```", "```go\ncode\n```"},
		{"inside a word", "it`s and don`t", "it`s and don`t"},
		{"inside a quoted value", `unknown command "a` + "`" + `b" for "astro"`, `unknown command "a` + "`" + `b" for "astro"`},
		{"a quoted value then a span", `unknown command "a` + "`" + `b". Use ` + "`astro local`", `unknown command "a` + "`" + `b". Use astro local`},
		{"a lone backtick quoted", `value "` + "`" + `" is not valid`, `value "` + "`" + `" is not valid`},
		{"across a newline", "a `first\nsecond` b", "a `first\nsecond` b"},
		{"padded with spaces", "a ` spaced ` b", "a ` spaced ` b"},
		{"a word after the closer", "`a`b and `c`", "`a`b and c"},
		{"longer than a span", "`" + string(bytes.Repeat([]byte("x"), maxSpanRunes+1)) + "`", "`" + string(bytes.Repeat([]byte("x"), maxSpanRunes+1)) + "`"},
		{"non-ASCII around", "é`astro`é, but «`astro`»", "é`astro`é, but «astro»"},
	}
	for _, c := range cases {
		s.Equal(c.want, StripBackticks(c.in), c.name)
	}
}

// On a terminal a span is bold and loses its backticks; the text around it
// is untouched.
func (s *Suite) TestBackticksBoldInAColoredPalette() {
	p := Palette{aurora.NewAurora(true)}
	got := p.Backticks("Use `astro local start` instead, not `x`y.")
	s.Equal("Use \x1b[1mastro local start\x1b[0m instead, not `x`y.", got)
	s.Equal(len("Use astro local start instead, not `x`y."), VisibleWidth(got))

	plain := Palette{aurora.NewAurora(false)}
	s.Equal("Use astro local start instead.", plain.Backticks("Use `astro local start` instead."))
}

// A writer that is not a terminal (a buffer, a pipe, a file) gets plain text,
// and so does a terminal with color turned off.
func (s *Suite) TestBackticksFollowTheWriter() {
	for _, v := range []string{cliColorForce, "CLICOLOR", "NO_COLOR"} {
		s.T().Setenv(v, "")
		s.Require().NoError(os.Unsetenv(v))
	}
	var buf bytes.Buffer
	s.Equal("run astro login", Backticks(&buf, "run `astro login`"))
	_, err := Fprintf(&buf, "run `%s` first\n", "astro login")
	s.Require().NoError(err)
	s.Equal("run astro login first\n", buf.String())

	s.T().Setenv(cliColorForce, "1")
	s.Equal("run \x1b[1mastro login\x1b[0m", Backticks(&buf, "run `astro login`"), "CLICOLOR_FORCE colors any writer")

	s.T().Setenv(cliColorForce, "")
	s.Require().NoError(os.Unsetenv(cliColorForce))
	s.T().Setenv("NO_COLOR", "1")
	s.False(shouldColor(func() bool { return true }), "NO_COLOR turns color off on a terminal")
}
