package env

import (
	"bytes"
	"os"
	"path/filepath"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
)

func (s *Suite) TestParseDotenvRoundTripsExport() {
	// Round-trip: write a set of values via the dotenv writer, read them back
	// via the parser, assert byte-identical values. Covers every escape path
	// dotenvQuote produces (whitespace, newline, quote, $, #, \, export prefix).
	objs := []astrocore.EnvironmentObject{
		{ObjectKey: "PLAIN", EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: "simple"}},
		{ObjectKey: "WITH_SPACES", EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: "two words"}},
		{ObjectKey: "WITH_NL", EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: "line1\nline2"}},
		{ObjectKey: "WITH_QUOTE", EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: `say "hi"`}},
		{ObjectKey: "WITH_DOLLAR", EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: "user $HOME"}},
		{ObjectKey: "WITH_HASH", EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: "v#oops"}},
		{ObjectKey: "WITH_BS", EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: `path\to\thing`}},
		{ObjectKey: "WITH_EXPORT", EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: "export ME"}},
	}
	var buf bytes.Buffer
	s.NoError(WriteVarList(objs, FormatDotenv, true, &buf))

	dir := s.T().TempDir()
	path := filepath.Join(dir, ".env")
	s.NoError(os.WriteFile(path, buf.Bytes(), 0o600))

	parsed, err := ParseDotenvFile(path)
	s.NoError(err)
	for i := range objs {
		o := &objs[i]
		s.Equal(o.EnvironmentVariable.Value, parsed[o.ObjectKey], "mismatch for %s", o.ObjectKey)
	}
}

func (s *Suite) TestParseDotenvFileMissing() {
	_, err := ParseDotenvFile("/does/not/exist/.env")
	s.Error(err)
	s.ErrorContains(err, "opening")
}

func (s *Suite) TestParseDotenvSkipsCommentsAndBlanks() {
	dir := s.T().TempDir()
	path := filepath.Join(dir, ".env")
	body := []byte("# leading comment\n\nFOO=bar\n  # indented comment treated as blank by godotenv\nBAZ=qux\n")
	s.NoError(os.WriteFile(path, body, 0o600))

	parsed, err := ParseDotenvFile(path)
	s.NoError(err)
	s.Equal("bar", parsed["FOO"])
	s.Equal("qux", parsed["BAZ"])
	s.Len(parsed, 2)
}

func (s *Suite) TestParseDotenvFromStdin() {
	r, w, err := os.Pipe()
	s.NoError(err)
	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	go func() {
		_, _ = w.WriteString("FOO=bar\nBAZ=qux\n")
		_ = w.Close()
	}()

	parsed, err := ParseDotenvFile("-")
	s.NoError(err)
	s.Equal("bar", parsed["FOO"])
	s.Equal("qux", parsed["BAZ"])
}

func (s *Suite) TestParseDotenvDuplicateKeyLastWins() {
	dir := s.T().TempDir()
	path := filepath.Join(dir, ".env")
	s.NoError(os.WriteFile(path, []byte("FOO=first\nFOO=second\n"), 0o600))

	parsed, err := ParseDotenvFile(path)
	s.NoError(err)
	s.Equal("second", parsed["FOO"])
}
