// Package shipcontext surveys what an image build that takes a project as its
// Docker context will carry: the paths the ignore rules leave in, the DAG
// files that will reach the image, and the files git ignores that will ship
// anyway, with those that look like credentials singled out. A deploy and
// `astro package astro` both read one survey, made in one walk, before they
// build.
package shipcontext

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/git"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// Options says what to survey.
type Options struct {
	// Ignore is the text of the ignore file the build reads.
	Ignore string
	// Digest, when set, receives each shipped path's name, kind, executable
	// bit, and contents or link target, in walk order: a content address of
	// what the build copies.
	Digest io.Writer
	// Git asks for the files git ignores among those that ship.
	Git bool
}

// Survey is what a build of a project context will carry.
type Survey struct {
	// Files are the regular files and symlinks that ship, slash-separated
	// and project-relative, in walk order.
	Files []string
	// DagsOnDisk counts the DAG files (.py, as the 1.x CLI counted them)
	// under dags/ on disk, links followed; DagsShipped those that reach the
	// image as files it can read (see Take).
	DagsOnDisk, DagsShipped int
	// Gitignored are the shipping files git ignores, and Secrets those of
	// them that look like credentials (LooksSecret). Both are empty outside a
	// git work tree, or when git fails.
	Gitignored, Secrets []string
}

// Take surveys the project at dir under opts, walking it once
// (scaffold.WalkContext): excluded directories are not entered, and symlinks
// are not followed, as docker copies them.
//
// A DAG reaches the image when it is a shipping regular .py file under
// dags/, or a shipping symlink under dags/ (dags/ itself included) whose
// target does. Docker copies a link as its text, so in the image the link
// resolves from where it sits: it only leads somewhere when the text is
// relative and, read from the link's directory, stays inside the project, at
// a path that ships too. An absolute link, or one climbing out, dangles in the
// image, and the DAGs it reaches on disk do not count.
func Take(dir string, opts Options) (Survey, error) {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	var s Survey
	kinds := map[string]fs.FileMode{}
	links := map[string]string{}
	err := scaffold.WalkContext(dir, "", opts.Ignore, func(rel string, d fs.DirEntry) error {
		slash := filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()
		kinds[slash] = mode.Type()
		if !mode.IsDir() {
			s.Files = append(s.Files, slash)
		}
		if mode&fs.ModeSymlink != 0 {
			target, err := os.Readlink(filepath.Join(dir, rel))
			if err != nil {
				return err
			}
			links[slash] = filepath.ToSlash(target)
		}
		if opts.Digest != nil {
			return digestEntry(opts.Digest, filepath.Join(dir, rel), slash, mode, links[slash])
		}
		return nil
	})
	if err != nil {
		return Survey{}, fmt.Errorf("reading the project's files: %w", err)
	}
	s.DagsShipped = countShippedDags(kinds, links)
	if s.DagsOnDisk, err = countDagsOnDisk(dir); err != nil {
		return Survey{}, err
	}
	if opts.Git {
		s.Gitignored = git.CheckIgnored(dir, s.Files)
		for _, f := range s.Gitignored {
			if LooksSecret(f) {
				s.Secrets = append(s.Secrets, f)
			}
		}
	}
	return s, nil
}

// countShippedDags counts the DAG files that reach the image readable: the
// regular .py files under dags/, and those reached through a link under
// dags/ (or dags/ itself) that resolves, in the image, to a shipping .py file
// or a shipping directory, whose regular .py files then count.
func countShippedDags(kinds map[string]fs.FileMode, links map[string]string) int {
	underDags := func(p string) bool { return p == "dags" || strings.HasPrefix(p, "dags/") }
	n := 0
	for p, kind := range kinds {
		if kind.IsRegular() && underDags(p) && strings.HasSuffix(p, ".py") {
			n++
		}
	}
	for p, text := range links {
		if !underDags(p) {
			continue
		}
		target, ok := linkTarget(p, text)
		if !ok {
			continue
		}
		kind, ships := kinds[target]
		switch {
		case !ships:
		case kind.IsRegular() && strings.HasSuffix(target, ".py") && !underDags(target):
			// A file under dags/ counts once, as itself.
			n++
		case kind.IsDir():
			for q, k := range kinds {
				if k.IsRegular() && strings.HasSuffix(q, ".py") && strings.HasPrefix(q, target+"/") && !underDags(q) {
					n++
				}
			}
		}
	}
	return n
}

// linkTarget is where a link at p (project-relative, slash-separated) with
// the given text leads in the image, as a project-relative path; ok is false
// for an absolute link or one that climbs out of the project.
func linkTarget(p, text string) (string, bool) {
	if text == "" || path.IsAbs(text) || filepath.IsAbs(filepath.FromSlash(text)) {
		return "", false
	}
	target := path.Clean(path.Join(path.Dir(p), text))
	if target == "." || target == ".." || strings.HasPrefix(target, "../") {
		return "", false
	}
	return target, true
}

// countDagsOnDisk counts the .py files under dir/dags, links followed, so an
// empty dags/ can be told from one whose DAGs do not ship.
func countDagsOnDisk(dir string) (int, error) {
	root, err := filepath.EvalSymlinks(filepath.Join(dir, "dags"))
	if err != nil {
		return 0, nil
	}
	n := 0
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasSuffix(p, ".py") {
			if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
				n++
			}
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("reading the project's dags/: %w", err)
	}
	return n, nil
}

// digestEntry writes one path's name, kind, executable bit and contents. Only
// what git keeps and docker copies counts: the executable bit and not the
// rest of the mode, which a umask or a checkout changes for the same commit;
// a link's text, not what it leads to.
func digestEntry(h io.Writer, full, rel string, mode fs.FileMode, link string) error {
	switch {
	case mode.IsDir():
		fmt.Fprintf(h, "dir\x00%s\x00", rel)
	case mode&fs.ModeSymlink != 0:
		fmt.Fprintf(h, "link\x00%s\x00%s\x00", rel, link)
	case mode.IsRegular():
		fmt.Fprintf(h, "file\x00%s\x00%t\x00", rel, mode.Perm()&0o111 != 0)
		f, err := os.Open(full)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		_, err = io.WriteString(h, "\x00")
		return err
	}
	return nil
}
