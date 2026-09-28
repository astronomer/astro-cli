package manifest

import "strings"

// Requirements are [project] dependencies as a requirements file has to state
// them for an install that cannot read [tool.uv.sources]: a generated image's
// requirements.txt, or a check's scratch environment. A dependency whose
// source is a git repository or a URL becomes a PEP 508 direct reference to
// it, the requirement uv itself resolves the source to. Any other dependency
// is unchanged.
func (m *Manifest) Requirements() []string {
	if len(m.UV.DirectURLs) == 0 {
		return m.Project.Dependencies
	}
	out := make([]string, len(m.Project.Dependencies))
	for i, dep := range m.Project.Dependencies {
		out[i] = directReference(dep, m.UV.DirectURLs)
	}
	return out
}

// directReference writes dep as `name[extras] @ url ; marker`. A version
// specifier is dropped, since a direct reference cannot carry one, and a
// dependency that is already a direct reference is left alone.
func directReference(dep string, urls map[string]string) string {
	address, ok := urls[DistName(dep)]
	if !ok {
		return dep
	}
	head, marker, hasMarker := strings.Cut(dep, ";")
	if strings.Contains(head, "@") {
		return dep
	}
	head = strings.TrimSpace(head)
	end := strings.IndexAny(head, " \t[<>=!~(")
	if end < 0 {
		end = len(head)
	}
	name, rest := head[:end], strings.TrimSpace(head[end:])
	if strings.HasPrefix(rest, "[") {
		if bracket := strings.Index(rest, "]"); bracket >= 0 {
			name += rest[:bracket+1]
		}
	}
	out := name + " @ " + address
	if hasMarker {
		out += " ; " + strings.TrimSpace(marker)
	}
	return out
}

// directURL is the PEP 508 URL a [tool.uv.sources] entry stands for, when it
// names a git repository or a URL. An entry uv limits with a marker, an extra
// or a group applies only to some installs, so it has none, and neither has a
// path or a workspace member, which only exist beside the project.
func directURL(source map[string]any) (string, bool) {
	for _, key := range []string{"marker", "extra", "group"} {
		if _, ok := source[key]; ok {
			return "", false
		}
	}
	subdirectory, _ := source["subdirectory"].(string)
	if repo, ok := source["git"].(string); ok && repo != "" {
		address := repo
		if !strings.HasPrefix(address, "git+") {
			address = "git+" + address
		}
		for _, key := range []string{"rev", "tag", "branch"} {
			if ref, ok := source[key].(string); ok && ref != "" {
				address += "@" + ref
				break
			}
		}
		return withSubdirectory(address, subdirectory), true
	}
	if address, ok := source["url"].(string); ok && address != "" {
		return withSubdirectory(address, subdirectory), true
	}
	return "", false
}

func withSubdirectory(address, subdirectory string) string {
	if subdirectory == "" {
		return address
	}
	return address + "#subdirectory=" + subdirectory
}
