package envresolve

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/astronomer/astro-cli/pkg/envschema"
)

// Every TOML block in the manifest reference parses.
//
// A doc example that does not load is worse than no example: it is the thing a
// user copy-pastes. Only the [tool.astro.env] blocks are extracted, since that
// is the section the parser reads; a block naming any other table is skipped
// rather than half-parsed.
//
// It lives in the root module rather than beside the parser in pkg/envschema,
// because the file it asserts about does. A sub-module's published zip contains
// only files under its own directory, so from there this reads a path that does
// not exist outside a repo checkout — and a guard connecting docs to code is
// the wrong thing to have skip quietly.
func TestManifestReferenceExamplesParse(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "docs", "manifest-reference.md"))
	if err != nil {
		t.Fatal(err)
	}

	blocks := 0
	for _, block := range strings.Split(string(body), "```toml") {
		snippet, _, ok := strings.Cut(block, "```")
		if !ok || !strings.Contains(snippet, "[tool.astro.env") {
			continue
		}
		blocks++
		t.Run(fmt.Sprintf("block-%d", blocks), func(t *testing.T) {
			if _, err := envschema.ParseSchema(decodeDocEnv(t, snippet)); err != nil {
				t.Errorf("a documented example does not parse:\n%s\nerror: %v", snippet, err)
			}
		})
	}
	if blocks == 0 {
		t.Fatal("found no [tool.astro.env] examples; this test would pass vacuously")
	}
}

// decodeDocEnv parses a [tool.astro.env] body the way pkg/manifest hands it
// over: decoded, untyped plain data.
//
// A snippet declaring other sections would fail this narrow struct rather than
// the grammar; the env blocks in that page stand alone, and the test asserts
// that stays true.
func decodeDocEnv(t *testing.T, body string) map[string]any {
	t.Helper()
	var f struct {
		Tool struct {
			Astro struct {
				Env map[string]any `toml:"env"`
			} `toml:"astro"`
		} `toml:"tool"`
	}
	if err := toml.Unmarshal([]byte(body), &f); err != nil {
		t.Fatal(err)
	}
	return f.Tool.Astro.Env
}
