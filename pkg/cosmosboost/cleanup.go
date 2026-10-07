package cosmosboost

import (
	"fmt"
	"path/filepath"
)

// CleanupReport is what Cleanup did: the roots it scanned, as given but
// absolute, the artifacts it removed, and the ones it kept because their
// producer marker is not this tool's. Every path is absolute. A root that is
// a symlink is scanned at its target, so the artifacts under it are reported
// at their resolved paths.
type CleanupReport struct {
	Roots   []string
	Removed []string
	Kept    []string
}

// Cleanup removes the Cosmos Boost artifacts under the given roots (default
// "."). It fails when any artifact it owns could not be removed. It does not
// report the result: its caller renders the CleanupReport it returns.
func Cleanup(roots ...string) (CleanupReport, error) {
	return cleanup(roots, filepath.Abs)
}

// cleanup is Cleanup with the way a root is made absolute handed in, so a
// test can make that fail without swapping a package variable. It fails only
// when the working directory cannot be read, which no platform reproduces
// the same way (Linux fails once the directory is removed; macOS still
// answers).
func cleanup(roots []string, abs func(string) (string, error)) (CleanupReport, error) {
	if len(roots) == 0 {
		roots = []string{"."}
	}
	given, walk, err := resolveRoots(roots, abs)
	if err != nil {
		return CleanupReport{}, err
	}
	summary, err := cleanupRoots(walk)
	if err != nil {
		return CleanupReport{}, err
	}
	report := CleanupReport{Roots: given}
	for _, r := range summary.Results {
		if r.Kept {
			report.Kept = append(report.Kept, r.Path)
		} else {
			report.Removed = append(report.Removed, r.Path)
		}
	}
	return report, nil
}

// resolveRoots is each root as given made absolute (given, what the report
// names), and the directory it resolves to with symlinks followed (walk, what
// is scanned), each directory once however it was spelled: `. $(pwd)`, or a
// symlink and its target. Walking the resolved path is what makes a symlinked
// root work at all: the walk does not descend a root that is itself a link.
//
// A root that cannot be made absolute fails the cleanup, since the report
// could not say where it removed anything; one that cannot be resolved (it
// does not exist) fails naming the root as it was typed.
func resolveRoots(roots []string, abs func(string) (string, error)) (given, walk []string, err error) {
	seen := map[string]bool{}
	for _, root := range roots {
		a, err := abs(root)
		if err != nil {
			return nil, nil, fmt.Errorf("removing the Cosmos Boost artifacts: resolving %q: %w", root, err)
		}
		resolved, err := filepath.EvalSymlinks(a)
		if err != nil {
			return nil, nil, fmt.Errorf("removing the Cosmos Boost artifacts: scanning %q: %w", root, err)
		}
		if seen[resolved] {
			continue
		}
		seen[resolved] = true
		given = append(given, a)
		walk = append(walk, resolved)
	}
	return given, walk, nil
}
