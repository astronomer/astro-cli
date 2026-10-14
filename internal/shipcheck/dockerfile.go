package shipcheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
)

// copiesContext reports whether the Dockerfile at path puts its whole build
// context into some stage: a COPY or ADD whose source is the context's root
// (`.`, `./`, `*`, `./*`) and not another stage's (--from), or a FROM on an
// Astro Runtime image, whose ONBUILD `COPY . .` copies it. A file that cannot
// be read counts as copying, so an unreadable one is held to the stricter
// rule; the build reports it anyway.
func copiesContext(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	for _, line := range instructions(string(data)) {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch strings.ToUpper(fields[0]) {
		case "FROM":
			if airflowrt.IsAstroRuntimeImage(imageName(fromImage(fields[1:]))) {
				return true
			}
		case "COPY", "ADD":
			if copiesRoot(strings.TrimSpace(line[len(fields[0]):])) {
				return true
			}
		}
	}
	return false
}

// instructions splits a Dockerfile into its instructions: comments and blank
// lines dropped, backslash continuations joined.
func instructions(text string) []string {
	var out []string
	var cur strings.Builder
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "#") || (line == "" && cur.Len() == 0) {
			continue
		}
		if body, ok := strings.CutSuffix(line, "\\"); ok {
			cur.WriteString(body)
			cur.WriteByte(' ')
			continue
		}
		cur.WriteString(line)
		out = append(out, cur.String())
		cur.Reset()
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// fromImage is the image reference of a FROM's arguments, past its flags.
func fromImage(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "--") {
			return a
		}
	}
	return ""
}

// imageName is ref without its tag or digest.
func imageName(ref string) string {
	if at := strings.Index(ref, "@"); at >= 0 {
		ref = ref[:at]
	}
	if colon := strings.LastIndex(ref, ":"); colon > strings.LastIndex(ref, "/") {
		ref = ref[:colon]
	}
	return ref
}

// copiesRoot reports whether a COPY or ADD's arguments (after the
// instruction) copy the context's root from the context.
func copiesRoot(args string) bool {
	var parts []string
	for _, f := range strings.Fields(args) {
		if strings.HasPrefix(f, "--") {
			if strings.HasPrefix(f, "--from") {
				return false // another stage or image, not the context
			}
			continue
		}
		parts = append(parts, f)
	}
	if rest := strings.Join(parts, " "); strings.HasPrefix(rest, "[") {
		var form []string
		if json.Unmarshal([]byte(rest), &form) != nil {
			return false
		}
		parts = form
	}
	if len(parts) < 2 {
		return false
	}
	for _, src := range parts[:len(parts)-1] {
		switch src {
		case ".", "./", "*", "./*":
			return true
		}
	}
	return false
}

// credentialURL matches a URL carrying a user and a password.
var credentialURL = regexp.MustCompile(`://[^/@\s]*:[^/@\s]*@`)

// vcsConfigHoldsCredential reports whether the git metadata at dir (a .git
// directory) has a config holding a credential: an http extraheader, which
// CI checkouts use for a token, or a remote URL with a password. A .git file
// (a submodule's, a worktree's) points elsewhere and holds no config.
func vcsConfigHoldsCredential(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "config"))
	if err != nil {
		return false
	}
	text := string(data)
	return strings.Contains(strings.ToLower(text), "extraheader") || credentialURL.MatchString(text)
}
