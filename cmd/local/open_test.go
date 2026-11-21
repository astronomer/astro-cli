package local

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

type runningRuntime struct{ fakeRuntime }

func (runningRuntime) ReadStatus(path string) (localrt.Status, error) {
	return localrt.Status{ProjectPath: path, State: localrt.StateRunning, Port: 8080}, nil
}

// `astro local open` opens the browser for a person at a terminal and says
// so; --print and --no-open open nothing and print the URL on stdout.
func TestOpenOpensOnlyWhenAsked(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		wantOpened bool
	}{
		{"default", []string{"local", "open"}, true},
		{"--no-open", []string{"local", "open", "--no-open"}, false},
		{"--print", []string{"local", "open", "--print"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, stdout := testDeps(t)
			d.Runtime = runningRuntime{}
			project := t.TempDir()
			if err := os.WriteFile(filepath.Join(project, "pyproject.toml"), []byte("[project]\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			d.WorkingDir = func() (string, error) { return project, nil }
			var opened []string
			d.OpenURL = func(u string) error { opened = append(opened, u); return nil }

			if err := execute(t, d, tc.args...); err != nil {
				t.Fatal(err)
			}
			if tc.wantOpened {
				if len(opened) != 1 {
					t.Fatalf("opened %v, want one URL", opened)
				}
				if !strings.Contains(stderrOf(d), opened[0]) {
					t.Errorf("stderr %q does not show the URL %q", stderrOf(d), opened[0])
				}
				if stdout.Len() != 0 {
					t.Errorf("stdout %q, want nothing when it opens", stdout.String())
				}
				return
			}
			if len(opened) != 0 {
				t.Errorf("opened %v, want nothing", opened)
			}
			if !strings.Contains(stdout.String(), "http") {
				t.Errorf("stdout %q has no URL", stdout.String())
			}
		})
	}
}
