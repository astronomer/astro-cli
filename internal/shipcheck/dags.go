package shipcheck

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
)

// dagsDir is the directory a Dags-only deploy uploads, on Astro and on Astro
// Private Cloud alike.
const dagsDir = "dags"

// CheckDags holds a Dags-only deploy of the project at dir to what an image
// deploy is held to (Check): a file the upload would carry that looks like a
// credential and that git does not track, or one reached through a link out of
// the project, refuses it. A committed one ships, as it does in an image. What
// git could not answer for is warned about through warn. An image deploy is
// checked by Survey before its build; a Dags-only one builds nothing and
// uploads dags/ as it is, so without this a stray .env or key file there
// reached the Deployment with no word said.
//
// The files are the ones both uploads walk (dagsFiles). A project with no
// dags/ is checked for nothing: the upload says so itself.
func CheckDags(dir string, warn func(string)) error {
	files, links, err := dagsFiles(dir)
	if err != nil {
		return fmt.Errorf("reading the project's %s/: %w", dagsDir, err)
	}
	if len(files) == 0 {
		return nil
	}
	r := Check(dir, &imagebuild.ProjectSurvey{Files: files, Links: links})
	if warn != nil {
		for _, u := range r.Unanswered {
			warn(fmt.Sprintf("could not ask git which files it tracks in %s, so files the Dags upload carries that look like credentials were not checked", u))
		}
		if n := len(r.Unchecked); n > 0 {
			warn(fmt.Sprintf("the Dags upload will carry %s, which git could not say %s committed: %s. If %s not belong on the Deployment, move %s out of %s/",
				agree(n, "this file that looks like credentials", "these files that look like credentials"), agree(n, "is", "are"),
				cappedList(r.Unchecked), agree(n, "it does", "they do"), agree(n, "it", "them"), dagsDir))
		}
	}
	return r.dagsErr()
}

// dagsErr refuses a Dags upload that would carry credentials, in the words a
// Dags upload calls for: it reads no ignore file, so the way out is to move
// the files, not to ignore them. nil when there are none.
func (r *Report) dagsErr() error {
	var why []string
	if n := len(r.Secrets); n > 0 {
		why = append(why, fmt.Sprintf("%s %s and %s not tracked by git",
			cappedList(r.Secrets), agree(n, "looks like credentials", "look like credentials"), agree(n, "is", "are")))
	}
	if n := len(r.Outside); n > 0 {
		why = append(why, fmt.Sprintf("%s %s and %s reached through a link out of the project",
			cappedList(r.Outside), agree(n, "looks like credentials", "look like credentials"), agree(n, "is", "are")))
	}
	if len(why) == 0 {
		return nil
	}
	it := agree(len(r.Secrets)+len(r.Outside), "it", "them")
	return fmt.Errorf("refusing to upload the Dags: %s, and the upload would carry %s to the Deployment. Move %s out of %s/, and try again",
		strings.Join(why, "; "), it, it, dagsDir)
}

// dagsFiles lists what a Dags-only upload of the project at dir carries, as
// project-relative slash paths under dags/, as both uploads walk it: a dags/
// that is a symlink is followed (and listed in links, so Check can tell where
// it leads), a link inside it is carried as a link rather than the file it
// points at and so is not listed, and bytecode caches are left out. A dags/
// that is missing, or a link that leads nowhere, carries nothing.
func dagsFiles(dir string) (files, links []string, err error) {
	top := filepath.Join(dir, dagsDir)
	info, err := os.Lstat(top)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	root := top
	if info.Mode()&fs.ModeSymlink != 0 {
		if root, err = filepath.EvalSymlinks(top); err != nil {
			return nil, nil, nil
		}
		links = []string{dagsDir}
	}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files = append(files, path.Join(dagsDir, filepath.ToSlash(rel)))
		return nil
	})
	return files, links, err
}
