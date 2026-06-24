package deploy

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/moby/patternmatcher"
	"github.com/moby/patternmatcher/ignorefile"

	"github.com/astronomer/astro-cli/config"
)

const deployIgnoreFileName = "deployignore"

// deployIgnoreFilePath is the project-relative path of the deploy ignore
// file, as shown in user-facing messages. Files matching its patterns
// (.dockerignore syntax) are excluded from deploy artifacts: the Docker
// build context, DAG bundles, and dbt bundles. Patterns are anchored at
// the project root, so files in a DAG bundle are matched with their dags/
// prefix (e.g. dags/internal_docs/).
var deployIgnoreFilePath = config.ConfigDir + "/" + deployIgnoreFileName

// readDeployIgnorePatterns parses the deploy ignore file at rootPath, if any.
// It returns nil patterns when the file does not exist.
func readDeployIgnorePatterns(rootPath string) ([]string, error) {
	patterns, err := readIgnorePatternsFile(filepath.Join(rootPath, config.ConfigDir, deployIgnoreFileName))
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", deployIgnoreFilePath, err)
	}
	return patterns, nil
}

// readIgnorePatternsFile parses a .dockerignore-syntax file into its patterns.
// It returns nil patterns when the file does not exist.
func readIgnorePatternsFile(filePath string) ([]string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	return ignorefile.ReadAll(f)
}

// deployIgnoreSkipFunc builds a predicate, suitable for fileutil.Tar, that
// reports whether a bundle-relative path matches the deploy ignore patterns at
// rootPath. It returns nil (include everything) when rootPath has no deploy
// ignore file.
//
// bundlePath is the directory being bundled. When it lies within rootPath,
// bundle-relative paths are re-anchored at rootPath before matching, so the
// patterns mean the same thing for every deploy type (e.g. dags/foo.md matches
// foo.md in a bundle of the dags directory). A bundlePath outside rootPath
// cannot be re-anchored, so its paths are matched relative to the bundle root.
//
// The predicate is consulted per file and fileutil.Tar never prunes
// directories; that is what makes "!" re-inclusion patterns work. If Tar ever
// learns to skip whole directories, it must replicate dockerignoreSkipFunc's
// pm.Exclusions() handling.
func deployIgnoreSkipFunc(rootPath, bundlePath string) (func(relPath string) bool, error) {
	patterns, err := readDeployIgnorePatterns(rootPath)
	if err != nil {
		return nil, err
	}
	if len(patterns) == 0 {
		return nil, nil
	}

	pm, err := patternmatcher.New(patterns)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern in %s: %w", deployIgnoreFilePath, err)
	}

	matchPrefix, err := bundleMatchPrefix(rootPath, bundlePath)
	if err != nil {
		return nil, err
	}

	return func(relPath string) bool {
		matchPath := relPath
		if matchPrefix != "" {
			matchPath = path.Join(matchPrefix, relPath)
		}
		matched, err := pm.MatchesOrParentMatches(matchPath)
		return err == nil && matched
	}, nil
}

// bundleMatchPrefix returns the rootPath-relative, slash-separated path of
// bundlePath, used to re-anchor bundle paths at the project root before
// matching. Both paths are made absolute first so a relative bundlePath (e.g.
// from --dags-path) resolves correctly. It returns "" when bundlePath is
// rootPath itself, or — with a warning, since patterns then match relative to
// the bundle root — when bundlePath lies outside rootPath.
func bundleMatchPrefix(rootPath, bundlePath string) (string, error) {
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return "", err
	}
	absBundle, err := filepath.Abs(bundlePath)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(absRoot, absBundle)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		fmt.Printf("Warning: %s is outside the project directory, so %s patterns are matched relative to it instead of the project root\n", bundlePath, deployIgnoreFilePath)
		return "", nil
	}
	if rel == "." {
		return "", nil
	}
	return filepath.ToSlash(rel), nil
}
