package pack

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// contextDigest is a digest of what a generated build copies from the project
// at dir into the image: every path the build's ignore file
// (imagebuild.ProjectIgnore) leaves in, with its kind, whether it is
// executable, and its bytes or link target. It also reports whether the
// ignore rules leave out every DAG file in dags/, which the package warns
// about.
//
// Only what git keeps and docker copies decides it: the executable bit and not
// the rest of the mode, which a umask or a checkout changes for the same
// commit; link targets, not what they point at, since docker copies a link as
// a link. The project is walked once (scaffold.WalkContext), and an excluded
// directory is not entered unless a "!" rule could match beneath it, so a
// virtualenv or a directory the user cannot read costs nothing and fails
// nothing.
func contextDigest(dir string) (digest string, dagsIgnored bool, err error) {
	ignore, err := imagebuild.ProjectIgnore(dir, nil)
	if err != nil {
		return "", false, err
	}
	h := sha256.New()
	err = scaffold.WalkContext(dir, "", ignore, func(rel string, d fs.DirEntry) error {
		return digestEntry(h, filepath.Join(dir, rel), filepath.ToSlash(rel), d)
	})
	if err != nil {
		return "", false, fmt.Errorf("reading the project's files: %w", err)
	}
	onDisk, shipped, err := scaffold.DagFiles(dir, ignore)
	if err != nil {
		return "", false, fmt.Errorf("reading the project's dags/: %w", err)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), onDisk > 0 && shipped == 0, nil
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
