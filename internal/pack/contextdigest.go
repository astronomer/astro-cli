package pack

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/moby/patternmatcher"
	"github.com/moby/patternmatcher/ignorefile"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
)

// contextDigest is a digest of what a generated build copies from the project
// at dir into the image: every path the build's ignore file
// (imagebuild.ProjectIgnore) leaves in, with its kind, whether it is
// executable, and its bytes or link target.
//
// Only what git keeps and docker copies decides it: the executable bit and not
// the rest of the mode, which a umask or a checkout changes for the same
// commit; link targets, not what they point at, since docker copies a link as
// a link. An excluded directory is not read, and with no "!" rule to bring
// anything back it is not even entered, so a virtualenv or a directory the
// user cannot read costs nothing and fails nothing.
func contextDigest(dir string) (string, error) {
	ignore, err := imagebuild.ProjectIgnore(dir, nil)
	if err != nil {
		return "", err
	}
	patterns, err := ignorefile.ReadAll(strings.NewReader(ignore))
	if err != nil {
		return "", fmt.Errorf("reading the project's .dockerignore: %w", err)
	}
	pm, err := patternmatcher.New(patterns)
	if err != nil {
		return "", fmt.Errorf("reading the project's .dockerignore: %w", err)
	}
	h := sha256.New()
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return walkErr
		}
		excluded, err := pm.MatchesOrParentMatches(rel)
		if err != nil {
			return err
		}
		if excluded {
			if walkErr == nil && d.IsDir() && !pm.Exclusions() {
				return filepath.SkipDir
			}
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		return digestEntry(h, path, filepath.ToSlash(rel), d)
	})
	if err != nil {
		return "", fmt.Errorf("reading the project's files: %w", err)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// digestEntry writes one path's name, kind, executable bit and contents.
func digestEntry(h io.Writer, path, rel string, d fs.DirEntry) error {
	info, err := d.Info()
	if err != nil {
		return err
	}
	mode := info.Mode()
	switch {
	case mode.IsDir():
		fmt.Fprintf(h, "dir\x00%s\x00", rel)
	case mode&fs.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "link\x00%s\x00%s\x00", rel, filepath.ToSlash(target))
	case mode.IsRegular():
		fmt.Fprintf(h, "file\x00%s\x00%t\x00", rel, mode.Perm()&0o111 != 0)
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		sum := sha256.New()
		if _, err := io.Copy(sum, f); err != nil {
			return err
		}
		fmt.Fprintf(h, "%x\x00", sum.Sum(nil))
	}
	return nil
}
