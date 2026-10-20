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

// dockerignoreRules are the virtualenv, git's metadata at any depth (a
// repository's config can hold a token in a remote URL or an extraheader),
// the files no shipped image carries (airflowrt.ShippedImageRules: the .env
// family, airflow_settings.yaml, and a root airflow.db, airflow.cfg and
// logs/), and localIgnoreRules. A generated image applies the same two
// airflowrt lists (imagebuild.ShippedImageIgnores) and leaves out a
// virtualenv and .git on its own.
var dockerignoreRules = slices.Concat([]string{".venv/", gitRule}, airflowrt.ShippedImageRules(), localIgnoreRules)

// gitRule leaves git's metadata out of a declared Dockerfile's context. init
// writes it, and a deploy or package names a .git its context carries
// (LocalFilesWarningFor), since that image is pushed. LocalFilesWarning, which
// `astro local start` runs, leaves it out of what it warns about: a local
// image goes nowhere, and a project converted before the rule existed should
// not start warning about it.
const gitRule = "**/.git"

// localWarnRules are dockerignoreRules without gitRule, the rules
// LocalFilesWarning warns about.
var localWarnRules = slices.DeleteFunc(slices.Clone(dockerignoreRules), func(r string) bool { return r == gitRule })

const dockerignoreHeader = "# Local secrets and per-machine files. Keep them out of the image.\n"

// dockerignoreIncludeHeader introduces the exceptions for the files the build's
// install step reads.
const dockerignoreIncludeHeader = "# Read by the build's install step (.astro/install-project.py). Keep them in the context.\n"

// dockerignoreLabel says what an update to an existing ignore file added.
func dockerignoreLabel(rules, includes bool) string {
	switch {
	case rules && includes:
		return "added the per-machine rules, and exceptions for the files the build reads"
	case includes:
		return "added exceptions for the files the build reads"
	default:
		return "added the per-machine rules"
	}
}

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
//
// A Dockerfile this run moved onto its -base tag also reads installInputs from
// the context, so an ignore file that leaves one out gains an exception for it,
// in the same append-only way the per-machine rules are added.
func planKeptDockerfileIgnore(dir string, from1x *project1x, cs *Changeset) error {
	if !declaresDockerfile(from1x) {
		return nil
	}
	var include []string
	if from1x.rewrite != nil {
		include = installInputs
	}
	ignore, err := planDockerignore(dir, fileDockerfile, include)
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
//
// include are files the build has to read: each one the file leaves out gets
// an exception (!path) under a header of its own, after the rules, since a
// later line wins.
func planDockerignore(dir, dockerfile string, include []string) (*Change, error) {
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
	var excluded []string
	for _, path := range include {
		if excludes(pm, path, false) {
			excluded = append(excluded, "!"+path)
		}
	}
	if len(missing) == 0 && len(excluded) == 0 {
		return nil, nil
	}
	var block string
	if len(missing) > 0 {
		block = dockerignoreHeader + strings.Join(missing, "\n") + "\n"
	}
	if len(excluded) > 0 {
		if block != "" {
			block += "\n"
		}
		block += dockerignoreIncludeHeader + strings.Join(excluded, "\n") + "\n"
	}
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
		Labels:  []string{path + " (" + dockerignoreLabel(len(missing) > 0, len(excluded) > 0) + ")"},
	}, nil
}

// LocalFilesWarning names the local and per-machine files a build of the
// project's own Dockerfile would copy into its image, and the ignore file to
// add them to, for `astro local start`: it does not name .git (see gitRule).
// dir is the project root and dockerfile the manifest's [tool.astro]
// dockerfile. It returns "" when there is nothing to say, and also when the
// ignore file cannot be read or parsed, since the build reports that itself.
func LocalFilesWarning(dir, dockerfile string) string {
	if dockerfile == "" {
		return ""
	}
	rel := dockerignorePath(dir, dockerfile)
	return localFilesMessage(dockerfile, filepath.ToSlash(rel), unignoredLocalFiles(dir, rel))
}

// LocalFilesWarningFor is the warning for an image that is pushed (a deploy's,
// a package's), over a listing already made of what the build's context
// carries: carried holds its paths, slash-separated and relative to the
// project (files, and version-control metadata as a path of its own), and
// ignoreFile is the ignore file the build reads, "" for none. It names .git
// too.
func LocalFilesWarningFor(dockerfile, ignoreFile string, carried []string) string {
	if dockerfile == "" {
		return ""
	}
	if ignoreFile == "" {
		ignoreFile = fileDockerignore
	}
	return localFilesMessage(dockerfile, ignoreFile, localFilesAmong(carried, dockerignoreRules))
}

func localFilesMessage(dockerfile, ignoreFile string, found []string) string {
	if len(found) == 0 {
		return ""
	}
	return fmt.Sprintf("the image built from %s would copy in these local or per-machine files: %s. To keep them out, add %s to %s",
		dockerfile, strings.Join(found, ", "), pronoun(len(found)), ignoreFile)
}

// ruleMatchers are rules, one matcher each, so a match can be put in rule
// order.
func ruleMatchers(rules []string) []*patternmatcher.PatternMatcher {
	out := make([]*patternmatcher.PatternMatcher, len(rules))
	for i, rule := range rules {
		pm, err := patternmatcher.New([]string{rule})
		if err != nil {
			panic(fmt.Sprintf("dockerignoreRules: %q: %v", rule, err)) // this package's own constants
		}
		out[i] = pm
	}
	return out
}

// localFilesAmong names, in rule order, the paths among carried that rules
// match: for each, the shallowest of it and its parents a rule matches, so a
// directory such as .venv is named rather than its files.
func localFilesAmong(carried, rules []string) []string {
	matchers := ruleMatchers(rules)
	found := make([][]string, len(matchers))
	seen := map[string]bool{}
	for _, p := range carried {
		parts := strings.Split(p, "/")
	prefixes:
		for n := 1; n <= len(parts); n++ {
			prefix := strings.Join(parts[:n], "/")
			for i, rule := range matchers {
				if ok, err := rule.MatchesOrParentMatches(prefix); err == nil && ok {
					if !seen[prefix] {
						seen[prefix] = true
						found[i] = append(found[i], prefix)
					}
					break prefixes
				}
			}
		}
	}
	return slices.Concat(found...)
}

// unignoredLocalFiles lists the paths under dir that match localWarnRules
// and that the ignore file at rel does not exclude (localFilesAmong). It walks
// the project, since some rules (**/.env) match at any depth, but does not
// enter a directory the ignore file excludes, one a rule names, or .git.
func unignoredLocalFiles(dir, rel string) []string {
	pm, _, err := readDockerignore(dir, rel)
	if err != nil {
		return nil
	}
	matchers := ruleMatchers(localWarnRules)
	var carried []string
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable path is the build's to report
		}
		r, err := filepath.Rel(dir, p)
		if err != nil || r == "." {
			return nil //nolint:nilerr // the root, or a path outside it
		}
		r = filepath.ToSlash(r)
		if d.Name() == ".git" || excludes(pm, r, d.IsDir()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			carried = append(carried, r)
			return nil
		}
		for _, rule := range matchers {
			if ok, err := rule.MatchesOrParentMatches(r); err == nil && ok {
				carried = append(carried, r)
				return fs.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		return nil
	}
	return localFilesAmong(carried, localWarnRules)
}
