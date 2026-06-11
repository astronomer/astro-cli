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

// DeployIgnoreFilePath is the project-relative path of the deploy ignore file.
// Files matching its patterns (.dockerignore syntax) are excluded from deploy
// artifacts: the Docker build context, DAG bundles, and dbt bundles. Patterns
// are anchored at the project root, so files in a DAG bundle are matched with
// their dags/ prefix (e.g. dags/internal_docs/).
var DeployIgnoreFilePath = filepath.Join(config.ConfigDir, "deployignore")

// readDeployIgnorePatterns parses the deploy ignore file at rootPath, if any.
// It returns nil patterns when the file does not exist.
func readDeployIgnorePatterns(rootPath string) ([]string, error) {
	f, err := os.Open(filepath.Join(rootPath, DeployIgnoreFilePath))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	patterns, err := ignorefile.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", DeployIgnoreFilePath, err)
	}
	return patterns, nil
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
		return nil, fmt.Errorf("invalid pattern in %s: %w", DeployIgnoreFilePath, err)
	}

	matchPrefix := ""
	if rel, err := filepath.Rel(rootPath, bundlePath); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		matchPrefix = filepath.ToSlash(rel)
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
