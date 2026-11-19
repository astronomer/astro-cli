package input

// Plain keeps the format characters that are part of text, and drops only
// what hides in it or reorders it. Every invisible character is written as an
// escape here, so the test source shows what it means.
func (s *Suite) TestPlain() {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"a family emoji keeps its joiners",
			"Delete \U0001F468\u200D\U0001F469\u200D\U0001F467 family?",
			"Delete \U0001F468\u200D\U0001F469\u200D\U0001F467 family?",
		},
		{
			"a Persian word keeps its non-joiner",
			"\u0645\u06CC\u200C\u062E\u0648\u0627\u0647\u06CC\u062F",
			"\u0645\u06CC\u200C\u062E\u0648\u0627\u0647\u06CC\u062F",
		},
		{
			"the soft hyphen and a variation selector stay",
			"co\u00ADoperate \u2764\uFE0F",
			"co\u00ADoperate \u2764\uFE0F",
		},
		{
			"a right-to-left override is stripped",
			"Delete run \u202Egnp.exe of x?",
			"Delete run gnp.exe of x?",
		},
		{
			"every bidi control is stripped",
			"a\u202A\u202B\u202C\u202D\u202E\u2066\u2067\u2068\u2069\u200E\u200F\u061Cb",
			"ab",
		},
		{
			"the zero-width space and the byte order mark are stripped",
			"\uFEFFye\u200Bs",
			"yes",
		},
		{
			"the word joiner and the other invisible operators are stripped",
			"y\u2060e\u2061s\u206Fx\uFFF9",
			"yesx",
		},
		{
			"a device control string is stripped with its payload",
			"x\x1bPqsixel-payload\x1b\\y\x1b_app\x1b\\z",
			"xyz",
		},
		{
			"escapes and control characters are stripped, spaces collapse",
			"n\x1b[2J\x1b]0;title\x07\x08  o\t\r\n",
			"n o",
		},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() { s.Equal(tc.want, Plain(tc.in)) })
	}
}

// Clean makes a name safe to print and changes nothing else: two names that
// differ only in their spacing stay different.
func (s *Suite) TestClean() {
	s.Equal("/p/my  proj", Clean("/p/my  proj"))
	s.NotEqual(Clean("/p/my  proj"), Clean("/p/my proj"))
	s.Equal(" a b ", Clean(" a b "))
	s.Equal("/p/proj", Clean("/p/pr\x1b]0;title\x07oj\n\t\u202E"))
	s.Equal("C:\\Users\\me\\proj", Clean("C:\\Users\\me\\proj"))
}

// Lines keeps a value's line breaks and tabs and drops what a terminal would act
// on: an escape sequence whole, any other control character, a bidi override.
func (s *Suite) TestLines() {
	cases := []struct {
		name, in, want string
	}{
		{"plain text is unchanged", "upstream key", "upstream key"},
		{"line breaks and tabs stay", "{\n\t\"a\": 1\n}", "{\n\t\"a\": 1\n}"},
		{"a CRLF line break becomes a newline", "one\r\ntwo", "one\ntwo"},
		{"a lone carriage return is dropped", "safe\rspoofed", "safespoofed"},
		{"a color escape is removed whole", "\x1b[31mred\x1b[0m", "red"},
		{"a clipboard write is removed whole", "note\x1b]52;c;aGk=\x07!", "note!"},
		{"a screen clear is removed whole", "\x1b[2J\x1b[Hfake prompt", "fake prompt"},
		{"a lone escape, a bell and a backspace are dropped", "a\x1bb\x07c\x08d", "abcd"},
		{"a C1 control is dropped", "a\u009bb", "ab"},
		{"a right-to-left override is dropped", "run \u202Egnp.exe", "run gnp.exe"},
		{"text in another script is unchanged", "caf\u00e9 \u65e5\u672c", "caf\u00e9 \u65e5\u672c"},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() { s.Equal(tc.want, Lines(tc.in)) })
	}
}

// Cell is one line with its own spacing kept, and nothing a terminal would act on.
func (s *Suite) TestCell() {
	cases := []struct{ name, in, want string }{
		{"plain text is unchanged", "my deployment", "my deployment"},
		{"its own spacing is kept", "two  spaces", "two  spaces"},
		{"a line break and a tab become spaces", "a\nb\tc", "a b c"},
		{"a CRLF becomes one space", "a\r\nb", "a b"},
		{"an escape sequence is removed whole", "prod\x1b]52;c;aGk=\x07\x1b[2J", "prod"},
		{"a bidi override is dropped", "run \u202Egnp.exe", "run gnp.exe"},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() { s.Equal(tc.want, Cell(tc.in)) })
	}
}
