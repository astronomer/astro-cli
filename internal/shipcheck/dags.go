package shipcheck

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/fileutil"
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
// src is the upload's own TarSource, and the files are the ones its tarball
// carries (dagsFiles): what is checked is what is uploaded. A project with no
// dags/ is checked for nothing: the upload says so itself.
func CheckDags(dir string, src fileutil.TarSource, warn func(string)) error {
	files, links, err := dagsFiles(dir, src)
	if err != nil {
		return fmt.Errorf("reading the project's Dags: %w", err)
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
// the files, not to ignore them, or, for one git does not track, to commit it
// when it is meant to ship (a committed file ships, as in an image). A file
// behind a link out of the project is refused committed or not, so it is only
// told to move. nil when there are none.
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
	way := fmt.Sprintf("Move %s out of %s/", it, dagsDir)
	if n := len(r.Secrets); n > 0 {
		way += fmt.Sprintf(", or commit the untracked %s if %s meant to ship", agree(n, "one", "ones"), agree(n, "it is", "they are"))
	}
	return fmt.Errorf("refusing to upload the Dags: %s, and the upload would carry %s to the Deployment. %s, and try again",
		strings.Join(why, "; "), it, way)
}

// dagsFiles lists the files a tarball of src carries (fileutil.WalkTar, the
// walk the upload writes it from), as project-relative slash paths under
// src.Dir, which is in the project at dir. A src.Dir that is a link is walked
// as the directory it leads to, and listed in links, so Check can tell where
// it leads; a link inside it is carried as a link rather than the file it
// points at, and so is not listed. A src.Dir that is not there, or a link that
// leads nowhere, carries nothing.
func dagsFiles(dir string, src fileutil.TarSource) (files, links []string, err error) {
	rel, err := filepath.Rel(dir, src.Dir)
	if err != nil || !filepath.IsLocal(rel) {
		return nil, nil, fmt.Errorf("%s is not in the project", src.Dir)
	}
	rel = filepath.ToSlash(rel)
	info, err := os.Lstat(src.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	root, err := filepath.EvalSymlinks(src.Dir)
	if err != nil {
		return nil, nil, nil
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		links = []string{rel}
	}
	err = fileutil.WalkTar(src, func(p, _ string, info os.FileInfo) error {
		if !info.Mode().IsRegular() {
			return nil
		}
		inside, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files = append(files, path.Join(rel, filepath.ToSlash(inside)))
		return nil
	})
	return files, links, err
}
