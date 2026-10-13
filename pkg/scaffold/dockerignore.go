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

	"github.com/astronomer/astro-cli/pkg/airflowrt"
)

// dockerignoreRules are the virtualenv, the files no shipped image carries
// (airflowrt.ShippedImageRules: the .env family, airflow_settings.yaml, and a
// root airflow.db, airflow.cfg and logs/), and localIgnoreRules. A generated
// image applies the same two airflowrt lists (imagebuild.ShippedImageIgnores)
// and leaves out a virtualenv on its own.
var dockerignoreRules = slices.Concat([]string{".venv/"}, airflowrt.ShippedImageRules(), localIgnoreRules)

const dockerignoreHeader = "# Local secrets and per-machine files. Keep them out of the image.\n"

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
func planKeptDockerfileIgnore(dir string, from1x *project1x, cs *Changeset) error {
	if !declaresDockerfile(from1x) {
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
	return fmt.Sprintf("the image built from %s would copy in these local or per-machine files: %s. To keep them out, add %s to %s",
		dockerfile, strings.Join(found, ", "), pronoun(len(found)), filepath.ToSlash(rel))
}

// unignoredLocalFiles lists the paths under dir that match dockerignoreRules
// and that the ignore file at rel does not exclude, slash-separated, in rule
// order. A directory a rule matches is named, not what is in it. It walks the
// project, since some rules (**/.env) match at any depth, but does not enter
// a directory the ignore file excludes, or version-control metadata.
func unignoredLocalFiles(dir, rel string) []string {
	pm, _, err := readDockerignore(dir, rel)
	if err != nil {
		return nil
	}
	rules := make([]*patternmatcher.PatternMatcher, len(dockerignoreRules))
	for i, rule := range dockerignoreRules {
		if rules[i], err = patternmatcher.New([]string{rule}); err != nil {
			return nil
		}
	}
	found := make([][]string, len(rules))
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable path is the build's to report
		}
		r, err := filepath.Rel(dir, p)
		if err != nil || r == "." {
			return nil //nolint:nilerr // the root, or a path outside it
		}
		r = filepath.ToSlash(r)
		if d.IsDir() && (d.Name() == ".git" || excludes(pm, r, true)) {
			return fs.SkipDir
		}
		for i, rule := range rules {
			if ok, err := rule.MatchesOrParentMatches(r); err != nil || !ok {
				continue
			}
			if !excludes(pm, r, d.IsDir()) {
				found[i] = append(found[i], r)
			}
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		return nil
	})
	if err != nil {
		return nil
	}
	return slices.Concat(found...)
}
