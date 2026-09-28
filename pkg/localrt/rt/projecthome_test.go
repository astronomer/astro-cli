package rt

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// canonicalTemp is a temp dir in its canonical spelling, so expected values
// compare byte for byte (macOS's /var is a symlink to /private/var).
func canonicalTemp(t *testing.T) string {
	t.Helper()
	d, err := CanonicalPath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func mkdirs(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	mkdirs(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// fakeWorktree lays out what `git worktree add` leaves behind, without git: the
// main repo's .git/worktrees/<name> with its commondir, and a .git file in the
// worktree pointing at it. gitdirLine is what the worktree's .git file says.
func fakeWorktree(t *testing.T, main, wt, name string, relativeGitdir bool) {
	t.Helper()
	admin := filepath.Join(main, ".git", "worktrees", name)
	writeFile(t, filepath.Join(admin, "commondir"), "../..\n")
	writeFile(t, filepath.Join(admin, "gitdir"), filepath.Join(wt, ".git")+"\n")
	gitdir := admin
	if relativeGitdir {
		rel, err := filepath.Rel(wt, admin)
		if err != nil {
			t.Fatal(err)
		}
		gitdir = rel
	}
	writeFile(t, filepath.Join(wt, ".git"), "gitdir: "+filepath.ToSlash(gitdir)+"\n")
}

func TestProjectHome(t *testing.T) {
	root := canonicalTemp(t)
	main := filepath.Join(root, "repo")
	mkdirs(t, filepath.Join(main, ".git"), filepath.Join(main, "services", "etl"))

	// A worktree inside the repo's own tree, one in a custom directory far away
	// (the desktop's WorktreeDir), and one whose .git file uses a relative gitdir.
	inside := filepath.Join(main, ".claude", "worktrees", "feat")
	custom := filepath.Join(root, "elsewhere", "wts", "feat2")
	relative := filepath.Join(root, "rel-wt")
	fakeWorktree(t, main, inside, "feat", false)
	fakeWorktree(t, main, custom, "feat2", false)
	fakeWorktree(t, main, relative, "rel", true)
	mkdirs(t, filepath.Join(custom, "services", "etl"))

	// A submodule: a .git file too, but its gitdir has no commondir.
	sub := filepath.Join(main, "vendor", "lib")
	writeFile(t, filepath.Join(main, ".git", "modules", "lib", "HEAD"), "ref: refs/heads/main\n")
	writeFile(t, filepath.Join(sub, ".git"), "gitdir: ../../.git/modules/lib\n")

	// A bare repository's worktree: commondir names a directory not called .git.
	bare := filepath.Join(root, "bare.git")
	bareWt := filepath.Join(root, "bare-wt")
	writeFile(t, filepath.Join(bare, "worktrees", "b", "commondir"), "../..\n")
	writeFile(t, filepath.Join(bareWt, ".git"), "gitdir: "+filepath.Join(bare, "worktrees", "b")+"\n")

	// A worktree whose main repo is gone, and a .git file that is not a pointer.
	orphan := filepath.Join(root, "orphan-wt")
	writeFile(t, filepath.Join(orphan, ".git"), "gitdir: "+filepath.Join(root, "gone", ".git", "worktrees", "x")+"\n")
	junk := filepath.Join(root, "junk")
	writeFile(t, filepath.Join(junk, ".git"), "not a pointer\n")

	outside := filepath.Join(root, "plain")
	mkdirs(t, outside)

	cases := []struct {
		name, dir, want string
	}{
		{"main worktree", main, main},
		{"monorepo subdirectory of the main worktree", filepath.Join(main, "services", "etl"), filepath.Join(main, "services", "etl")},
		{"worktree inside the repo", inside, main},
		{"worktree in a custom directory", custom, main},
		{"subdirectory of a worktree keeps its offset", filepath.Join(custom, "services", "etl"), filepath.Join(main, "services", "etl")},
		{"relative gitdir", relative, main},
		{"submodule is not a worktree", sub, sub},
		{"bare repository worktree", bareWt, bareWt},
		{"worktree of a missing repo", orphan, orphan},
		{".git file that is not a pointer", junk, junk},
		{"outside any repository", outside, outside},
	}
	for _, tc := range cases {
		got, err := ProjectHome(tc.dir)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: ProjectHome(%s) = %s, want %s", tc.name, tc.dir, got, tc.want)
		}
	}

	if _, err := ProjectHome(filepath.Join(root, "does-not-exist")); err == nil {
		t.Error("a directory that cannot be canonicalized must be an error")
	}
}

// A .git file is only a file in the checkout, so one written by hand, naming an
// admin directory whose commondir points at another project's .git, must not
// borrow that project's home. git's back-link (<gitdir>/gitdir) is what proves
// the admin directory belongs to this checkout.
func TestProjectHomeRejectsACraftedPointer(t *testing.T) {
	root := canonicalTemp(t)
	victim := filepath.Join(root, "victim")
	mkdirs(t, filepath.Join(victim, ".git"))

	crafted := filepath.Join(root, "crafted")
	admin := filepath.Join(root, "x")
	writeFile(t, filepath.Join(admin, "commondir"), filepath.Join(victim, ".git")+"\n")
	writeFile(t, filepath.Join(crafted, ".git"), "gitdir: "+filepath.ToSlash(admin)+"\n")

	// A back-link naming another checkout's .git is no better than none.
	wrongBack := filepath.Join(root, "crafted-wrong-back")
	admin2 := filepath.Join(root, "y")
	writeFile(t, filepath.Join(admin2, "commondir"), filepath.Join(victim, ".git")+"\n")
	writeFile(t, filepath.Join(admin2, "gitdir"), filepath.Join(victim, ".git")+"\n")
	writeFile(t, filepath.Join(wrongBack, ".git"), "gitdir: "+filepath.ToSlash(admin2)+"\n")

	for _, dir := range []string{crafted, wrongBack} {
		got, err := ProjectHome(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got != dir {
			t.Errorf("ProjectHome(%s) = %s, want its own path: the pointer has no back-link to it", dir, got)
		}
	}
}

// The same answers from what git itself writes, so the hand-built layout above
// cannot drift from the real one.
func TestProjectHomeAgainstRealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root := canonicalTemp(t)
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	lib := filepath.Join(root, "lib")
	main := filepath.Join(root, "repo")
	mkdirs(t, lib, main)
	run(lib, "init", "-q")
	run(lib, "commit", "-q", "--allow-empty", "-m", "init")
	run(main, "init", "-q")
	run(main, "commit", "-q", "--allow-empty", "-m", "init")
	run(main, "-c", "protocol.file.allow=always", "submodule", "add", "-q", lib, "vendor/lib")
	run(main, "commit", "-q", "-m", "submodule")
	wt := filepath.Join(root, "custom", "feature")
	run(main, "worktree", "add", "-q", wt)

	for dir, want := range map[string]string{
		main:                                 main,
		wt:                                   main,
		filepath.Join(main, "vendor", "lib"): filepath.Join(main, "vendor", "lib"),
	} {
		got, err := ProjectHome(dir)
		if err != nil {
			t.Fatalf("ProjectHome(%s): %v", dir, err)
		}
		if got != want {
			t.Errorf("ProjectHome(%s) = %s, want %s", dir, got, want)
		}
	}
}
