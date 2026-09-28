package airflowrt

import (
	"fmt"
	"os"
	"path"
	"strings"
)

// SecretMount is one `RUN --mount=type=secret` in a Dockerfile: the id the
// build looks the secret up by, and the line its RUN starts on.
type SecretMount struct {
	ID   string
	Line int
}

// SecretMounts lists the build secrets the Dockerfile at dockerfilePath
// mounts, in file order, one entry per id.
//
// It reads conservatively. An instruction continued over several lines with
// a trailing backslash is one instruction, and comment lines inside it are
// skipped, as docker skips them. Every --mount flag on a RUN counts, until the
// first word that is not a flag. A secret mount with no id takes its source,
// then the base name of its target, as docker does. An id built from a build
// argument is left out, because only the build knows its value. Every stage
// is read, including one the final stage does not need, and so is the body of
// a heredoc.
func SecretMounts(dockerfilePath string) ([]SecretMount, error) {
	data, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return nil, fmt.Errorf("error reading Dockerfile: %w", err)
	}
	var mounts []SecretMount
	seen := map[string]bool{}
	for _, in := range instructions(string(data)) {
		fields := strings.Fields(in.text)
		if len(fields) == 0 || !strings.EqualFold(fields[0], "RUN") {
			continue
		}
		for _, f := range fields[1:] {
			if !strings.HasPrefix(f, "--") {
				break
			}
			spec, ok := strings.CutPrefix(f, "--mount=")
			if !ok {
				continue
			}
			if id := secretMountID(spec); id != "" && !seen[id] {
				seen[id] = true
				mounts = append(mounts, SecretMount{ID: id, Line: in.line})
			}
		}
	}
	return mounts, nil
}

type instruction struct {
	text string
	line int
}

// instructions joins a Dockerfile's continued lines into one instruction
// each, with the line number the instruction starts on.
func instructions(data string) []instruction {
	var out []instruction
	var cur []string
	start := 0
	for i, raw := range strings.Split(data, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(cur) == 0 {
			start = i + 1
		}
		body, continued := strings.CutSuffix(line, `\`)
		cur = append(cur, body)
		if !continued {
			out = append(out, instruction{text: strings.Join(cur, " "), line: start})
			cur = nil
		}
	}
	if len(cur) > 0 {
		out = append(out, instruction{text: strings.Join(cur, " "), line: start})
	}
	return out
}

// secretMountID reads the id out of a --mount value, or "" when the mount is
// not a secret or its id comes from a build argument.
func secretMountID(spec string) string {
	var typ, id, source, target string
	for _, kv := range strings.Split(spec, ",") {
		k, v, _ := strings.Cut(kv, "=")
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "type":
			typ = v
		case "id":
			id = v
		case "source", "src":
			source = v
		case "target", "dst", "destination":
			target = v
		}
	}
	if typ != "secret" {
		return ""
	}
	if source != "" {
		id = source
	}
	if id == "" && target != "" {
		id = path.Base(target)
	}
	if strings.Contains(id, "$") {
		return ""
	}
	return id
}
