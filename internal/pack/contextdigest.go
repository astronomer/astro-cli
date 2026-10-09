package pack

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/git"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// contextDigest is a digest of what a generated build copies from the project
// at dir into the image: every path the build's ignore file
// (imagebuild.ProjectIgnore) leaves in, with its kind, whether it is
// executable, and its bytes or link target. It also returns what the package
// warns about those files: ignore rules that leave out every DAG file in
// dags/, and files git ignores that the image will carry.
//
// Only what git keeps and docker copies decides it: the executable bit and not
// the rest of the mode, which a umask or a checkout changes for the same
// commit; link targets, not what they point at, since docker copies a link as
// a link. The project is walked once (scaffold.WalkContext), and an excluded
// directory is not entered unless a "!" rule could match beneath it, so a
// virtualenv or a directory the user cannot read costs nothing and fails
// nothing. The DAG files are counted in the same pass; only when it finds none
// is dags/ looked at again, to tell an empty dags/ from rules that leave it out.
func contextDigest(dir string) (digest string, warnings []string, err error) {
	ignore, err := imagebuild.ProjectIgnore(dir, nil)
	if err != nil {
		return "", nil, err
	}
	h := sha256.New()
	var files []string
	dags := 0
	err = scaffold.WalkContext(dir, "", ignore, func(rel string, d fs.DirEntry) error {
		slash := filepath.ToSlash(rel)
		if !d.IsDir() {
			files = append(files, slash)
			if strings.HasPrefix(slash, "dags/") && strings.HasSuffix(slash, ".py") {
				dags++
			}
		}
		return digestEntry(h, filepath.Join(dir, rel), slash, d)
	})
	if err != nil {
		return "", nil, fmt.Errorf("reading the project's files: %w", err)
	}
	if dags == 0 {
		onDisk, shipped, err := scaffold.DagFiles(dir, ignore)
		if err != nil {
			return "", nil, fmt.Errorf("reading the project's dags/: %w", err)
		}
		if onDisk > 0 && shipped == 0 {
			warnings = append(warnings, dagsIgnoredWarning)
		}
	}
	if w := imagebuild.GitignoredWarning(git.CheckIgnored(dir, files)); w != "" {
		warnings = append(warnings, w)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), warnings, nil
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
