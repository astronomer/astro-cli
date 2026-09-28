// A declared Dockerfile builds with the whole project as its context, so only
// the ignore file keeps per-machine files out of an image that gets pushed to a
// registry. A generated build makes its own context and needs none of this.

package scaffold

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/moby/patternmatcher"
	"github.com/moby/patternmatcher/ignorefile"
)

// dockerignoreRules add the virtualenv and the local .env to localIgnoreRules.
var dockerignoreRules = slices.Concat([]string{".venv/", ".env"}, localIgnoreRules)

const dockerignoreHeader = "# Per-machine files Astro tools write into the project. Keep them out of the image.\n"

// dockerignorePath is the ignore file a build of dockerfile reads, relative to
// the context. BuildKit reads <Dockerfile>.dockerignore instead of the
// context's .dockerignore when it exists.
func dockerignorePath(dir, dockerfile string) string {
	own := filepath.FromSlash(dockerfile) + fileDockerignore
	if _, err := os.Stat(filepath.Join(dir, own)); err == nil {
		return own
	}
	return fileDockerignore
}

// readDockerignore returns a matcher for the ignore file at rel under dir, and
// its bytes. A missing file is an empty matcher and nil bytes.
func readDockerignore(dir, rel string) (*patternmatcher.PatternMatcher, []byte, error) {
	data, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}
	patterns, err := ignorefile.ReadAll(strings.NewReader(string(data)))
	if err != nil {
		return nil, nil, err
	}
	pm, err := patternmatcher.New(patterns)
	if err != nil {
		return nil, nil, err
	}
	return pm, data, nil
}

// excludes reports whether pm keeps path out of the context. A directory also
// counts when a rule such as dir/* or dir/** leaves out everything inside it.
func excludes(pm *patternmatcher.PatternMatcher, path string, isDir bool) bool {
	path = strings.TrimSuffix(path, "/")
	matched, err := pm.MatchesOrParentMatches(path)
	if err == nil && !matched && isDir {
		matched, err = pm.MatchesOrParentMatches(path + "/x")
	}
	return err == nil && matched
}

// planKeptDockerfileIgnore adds the .dockerignore change when init keeps the
// Dockerfile as the build, whose context is the whole project.
func planKeptDockerfileIgnore(dir string, v1 *v1Project, cs *Changeset) error {
	if !declaresDockerfile(v1) {
		return nil
	}
	ignore, err := planDockerignore(dir, fileDockerfile)
	if err != nil || ignore == nil {
		return err
	}
	cs.Changes = append(cs.Changes, *ignore)
	return nil
}

// planDockerignore works out the change that makes the ignore file a build of
// dockerfile reads leave out dockerignoreRules: the file itself when there is
// none, or the missing rules appended under one header, keeping the lines
// already there. It returns nil when nothing is missing.
//
// A rule is missing unless the file already excludes the rule's own spelling,
// so .venv, **/.venv or a blanket .astro count, and one named .local.yaml file
// does not.
func planDockerignore(dir, dockerfile string) (*Change, error) {
	rel := dockerignorePath(dir, dockerfile)
	pm, data, err := readDockerignore(dir, rel)
	if err != nil {
		return nil, err
	}
	var missing []string
	for _, rule := range dockerignoreRules {
		if !excludes(pm, rule, strings.HasSuffix(rule, "/")) {
			missing = append(missing, rule)
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}
	block := dockerignoreHeader + strings.Join(missing, "\n") + "\n"
	path := filepath.ToSlash(rel)
	if data == nil {
		return &Change{Kind: CreateFile, Path: path, Content: []byte(block), Labels: []string{path}}, nil
	}
	s := string(data)
	var b strings.Builder
	b.WriteString(s)
	if s != "" && !strings.HasSuffix(s, "\n") {
		b.WriteByte('\n')
	}
	if s != "" {
		b.WriteByte('\n')
	}
	b.WriteString(block)
	return &Change{
		Kind:    UpdateFile,
		Path:    path,
		Content: []byte(b.String()),
		Labels:  []string{path + " (added the per-machine rules)"},
	}, nil
}

// LocalFilesWarning names the per-machine files a build of the project's own
// Dockerfile would copy into its image, and the ignore file to add them to. dir
// is the project root and dockerfile the manifest's [tool.astro] dockerfile. It
// returns "" when there is nothing to say, and also when the ignore file cannot
// be read or parsed, since the build reports that itself.
func LocalFilesWarning(dir, dockerfile string) string {
	if dockerfile == "" {
		return ""
	}
	rel := dockerignorePath(dir, dockerfile)
	found := unignoredLocalFiles(dir, rel)
	if len(found) == 0 {
		return ""
	}
	return fmt.Sprintf("the image built from %s would copy in these per-machine files: %s. To keep them out, add %s to %s",
		dockerfile, strings.Join(found, ", "), pronoun(len(found)), filepath.ToSlash(rel))
}

// unignoredLocalFiles lists the paths under dir that match dockerignoreRules
// and that the ignore file at rel does not exclude, slash-separated, in rule
// order.
func unignoredLocalFiles(dir, rel string) []string {
	pm, _, err := readDockerignore(dir, rel)
	if err != nil {
		return nil
	}
	root := os.DirFS(dir)
	var found []string
	for _, rule := range dockerignoreRules {
		matches, err := fs.Glob(root, strings.TrimSuffix(rule, "/"))
		if err != nil {
			continue
		}
		for _, m := range matches {
			info, err := fs.Stat(root, m)
			if err == nil && !excludes(pm, m, info.IsDir()) {
				found = append(found, m)
			}
		}
	}
	return found
}
