package ide

import (
	"path/filepath"
	"testing"
)

// An export skips git metadata by the rule an import refuses it by: where the
// filesystem ignores case, .GIT is the .git directory, and on Windows so is
// .git. with a trailing dot. Elsewhere .GIT is an ordinary directory.
func TestInGitDirFoldsAsTheFilesystemDoes(t *testing.T) {
	sep := string(filepath.Separator)
	for _, tc := range []struct {
		rel  string
		goos string
		want bool
	}{
		{".git", "linux", true},
		{".git" + sep + "config", "linux", true},
		{".GIT" + sep + "config", "linux", false},
		{".github" + sep + "workflows", "linux", false},
		{".GIT", "darwin", true},
		{".Git" + sep + "HEAD", "darwin", true},
		{".gitignore", "darwin", false},
		{".GIT" + sep + "config", "windows", true},
		{".git." + sep + "config", "windows", true},
		{"dags" + sep + ".git", "darwin", false},
	} {
		if got := inGitDir(tc.rel, tc.goos); got != tc.want {
			t.Errorf("inGitDir(%q, %s) = %v, want %v", tc.rel, tc.goos, got, tc.want)
		}
	}
}
