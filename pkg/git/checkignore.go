package git

import (
	"bytes"
	"os/exec"
	"strings"
)

// CheckIgnored returns those of paths (relative to dir, in any separator git
// accepts) that the .gitignore rules of the repository holding dir ignore, in
// the order git reports them. Tracked files are never reported, since git does
// not apply ignore rules to them.
//
// It is advisory, so it never fails: when dir is not in a git work tree, git
// is not installed, or git errs, it returns nil.
func CheckIgnored(dir string, paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	if err := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		return nil
	}
	// NUL-separated both ways, so no file name can split or join an entry.
	// check-ignore exits 1 when nothing is ignored, and that answer is nil as
	// much as a failure is.
	cmd := exec.Command("git", "-C", dir, "check-ignore", "-z", "--stdin")
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil
	}
	var ignored []string
	for _, p := range strings.Split(out.String(), "\x00") {
		if p != "" {
			ignored = append(ignored, p)
		}
	}
	return ignored
}
