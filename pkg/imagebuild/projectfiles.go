package imagebuild

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/moby/patternmatcher"
	"github.com/moby/patternmatcher/ignorefile"
)

// The project directories an image can carry. ONBUILD `COPY . .` puts each
// under AIRFLOW_HOME (/usr/local/airflow), where Airflow reads dags/ and
// plugins/ and the project's code imports include/, as `astro local` mounts
// them.
const (
	DagsDir    = "dags"
	PluginsDir = "plugins"
	IncludeDir = "include"
)

// ProjectCode lists the project directories an image ships: plugins/ and
// include/ always, and dags/ when withDags is set. A Deployment that takes DAG
// uploads gets its DAGs that way instead of from the image.
func ProjectCode(withDags bool) []string {
	if withDags {
		return []string{DagsDir, PluginsDir, IncludeDir}
	}
	return []string{PluginsDir, IncludeDir}
}

// perMachineExcludes keep what a machine writes into a project out of every
// staged context, whatever the project's .dockerignore says: they are applied
// after its rules, so a "!" there cannot bring one back. The virtualenv and
// .env match pkg/scaffold's dockerignore rules, and the pickling fix is the
// plugin the standalone engine drops into plugins/ for Airflow 2 on macOS
// (scaffold's pickleFixRule).
var perMachineExcludes = []string{
	"**/.venv",
	"**/.env",
	"**/.git",
	"**/__pycache__",
	"**/*.pyc",
	".astro",
	PluginsDir + "/fix_local_executor_pickle.py",
}

// ignoreFile is the ignore file a generated build reads its project rules from,
// at the project root. There is no Dockerfile of the project's to have a
// <Dockerfile>.dockerignore of its own.
const ignoreFile = ".dockerignore"

// checkProjectFiles refuses a path that is not a plain project-relative one,
// or that would land on a file the generated build writes itself.
func checkProjectFiles(paths []string) error {
	for _, p := range paths {
		rel := filepath.FromSlash(p)
		if !filepath.IsLocal(rel) || filepath.Clean(rel) == "." {
			return fmt.Errorf("project file %q is not a path inside the project", p)
		}
		switch filepath.Clean(rel) {
		case requirementsName, packagesName:
			return fmt.Errorf("project file %q would replace the %s the build generates", p, filepath.Clean(rel))
		}
	}
	return nil
}

// projectMatcher reads the project's .dockerignore and adds the per-machine
// excludes after it. A missing ignore file contributes no rules.
func projectMatcher(projectDir string) (*patternmatcher.PatternMatcher, error) {
	var patterns []string
	f, err := os.Open(filepath.Join(projectDir, ignoreFile))
	switch {
	case err == nil:
		patterns, err = ignorefile.ReadAll(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", ignoreFile, err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("reading %s: %w", ignoreFile, err)
	}
	pm, err := patternmatcher.New(append(patterns, perMachineExcludes...))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", ignoreFile, err)
	}
	return pm, nil
}

// projectEntry is one path walkProjectFiles reports: rel is project-relative in
// the OS's separators, src the file to read, info its Lstat.
type projectEntry struct {
	rel  string
	src  string
	info fs.FileInfo
}

// walkProjectFiles visits every directory, file and symlink under the given
// project-relative paths that the project's .dockerignore and the per-machine
// excludes leave in, parents before children and in lexical order. A path that
// does not exist is skipped. A path that is itself a symlink is followed, so a
// dags/ linked from elsewhere ships what it points at; a symlink below one is
// reported as a link, as docker's context would carry it.
func walkProjectFiles(projectDir string, paths []string, visit func(projectEntry) error) error {
	if err := checkProjectFiles(paths); err != nil {
		return err
	}
	pm, err := projectMatcher(projectDir)
	if err != nil {
		return err
	}
	for _, p := range paths {
		rel := filepath.Clean(filepath.FromSlash(p))
		root, err := filepath.EvalSymlinks(filepath.Join(projectDir, rel))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("reading %s: %w", p, err)
		}
		err = filepath.WalkDir(root, func(src string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			sub, err := filepath.Rel(root, src)
			if err != nil {
				return err
			}
			name := filepath.Join(rel, sub)
			excluded, err := pm.MatchesOrParentMatches(name)
			if err != nil {
				return err
			}
			if excluded {
				// A "!" rule can bring back something below an excluded
				// directory, so descend when the rules have any.
				if d.IsDir() && !pm.Exclusions() {
					return filepath.SkipDir
				}
				return nil
			}
			info, err := os.Lstat(src)
			if err != nil {
				return err
			}
			return visit(projectEntry{rel: name, src: src, info: info})
		})
		if err != nil {
			return fmt.Errorf("reading %s: %w", p, err)
		}
	}
	return nil
}

// ownerRWX keeps a staged directory writable by the build while files are
// copied into it, whatever its mode in the project.
const ownerRWX = 0o700

// stageProjectFiles copies the project paths into the build context at
// contextDir, under the same relative names, so the runtime image's ONBUILD
// `COPY . .` copies them into AIRFLOW_HOME. Modes are kept (docker carries the
// executable bit into the image), and a symlink is recreated as one.
func stageProjectFiles(projectDir, contextDir string, paths []string) error {
	return walkProjectFiles(projectDir, paths, func(e projectEntry) error {
		dst := filepath.Join(contextDir, e.rel)
		mode := e.info.Mode()
		switch {
		case mode.IsDir():
			// Owner-writable while staging, so the files below can be written.
			if err := os.MkdirAll(dst, mode.Perm()|ownerRWX); err != nil {
				return err
			}
			return os.Chmod(dst, mode.Perm()|ownerRWX)
		case mode&fs.ModeSymlink != 0:
			target, err := os.Readlink(e.src)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(dst), contextDirPerm); err != nil {
				return err
			}
			return os.Symlink(target, dst)
		case mode.IsRegular():
			if err := os.MkdirAll(filepath.Dir(dst), contextDirPerm); err != nil {
				return err
			}
			return copyFile(e.src, dst, mode.Perm())
		default:
			// A socket or a device has nothing to ship.
			return nil
		}
	})
}

func copyFile(src, dst string, perm fs.FileMode) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	// OpenFile's mode is filtered by the umask; the image should carry the
	// project's.
	return os.Chmod(dst, perm)
}

// ProjectFilesDigest is a digest of what a build with these ProjectFiles would
// stage from projectDir: every name, mode, link target and file's bytes, under
// the same ignore rules. A caller that content-addresses an image (`astro
// package astro`'s tag) adds it, so editing a plugin moves the tag.
func ProjectFilesDigest(projectDir string, paths []string) (string, error) {
	h := sha256.New()
	err := walkProjectFiles(projectDir, paths, func(e projectEntry) error {
		mode := e.info.Mode()
		fmt.Fprintf(h, "%s\x00%s\x00", filepath.ToSlash(e.rel), mode.Type()|mode.Perm())
		switch {
		case mode&fs.ModeSymlink != 0:
			target, err := os.Readlink(e.src)
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "%s\x00", target)
		case mode.IsRegular():
			f, err := os.Open(e.src)
			if err != nil {
				return err
			}
			_, err = io.Copy(h, f)
			f.Close()
			if err != nil {
				return err
			}
			h.Write([]byte{0})
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
