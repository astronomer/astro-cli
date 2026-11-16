package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// Each warning is printed once per file and key, however often a run loads
// the file, in the words astro local start uses.
func TestManifestWarningPrinter(t *testing.T) {
	var buf bytes.Buffer
	show := manifestWarningPrinter(&buf)
	w := []manifest.Problem{{Code: manifest.CodeUnknownKey, Key: "tool.astro.palette", Reason: "unknown key, ignored"}}
	show("/p/pyproject.toml", w)
	show("/p/pyproject.toml", w)
	if got := strings.Count(buf.String(), "pyproject.toml: tool.astro.palette: unknown key, ignored"); got != 1 {
		t.Errorf("want the warning once, got %d in %q", got, buf.String())
	}
	show("/p/./sub/../pyproject.toml", w)
	if got := strings.Count(buf.String(), "tool.astro.palette"); got != 1 {
		t.Errorf("one file however it is spelled, got %q", buf.String())
	}
	show("/other/pyproject.toml", w)
	if got := strings.Count(buf.String(), "tool.astro.palette"); got != 2 {
		t.Errorf("another file's warning is its own, got %q", buf.String())
	}
}
