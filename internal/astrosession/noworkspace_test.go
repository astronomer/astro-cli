package astrosession

import (
	"errors"
	"strings"
	"testing"
)

func TestNoWorkspaceNamesTheAdviceOfTheCommand(t *testing.T) {
	t.Setenv(EnvAPIToken, "token")
	for _, hint := range []string{HintFlag, HintFlagOrProject, HintCurrent, "so write the id in the path"} {
		err := NoWorkspaceHint(nil, hint)
		if !errors.Is(err, ErrOrgTokenNoWorkspace) {
			t.Fatalf("hint %q: %v is not ErrOrgTokenNoWorkspace", hint, err)
		}
		if !strings.HasSuffix(err.Error(), ", "+hint) {
			t.Errorf("hint %q is not the error's advice: %q", hint, err)
		}
	}
	if err := NoWorkspace(nil); !strings.HasSuffix(err.Error(), HintFlag) {
		t.Errorf("NoWorkspace gives %q, want the flag advice", err)
	}
}

func TestNoWorkspaceWithoutATokenIsTheFallback(t *testing.T) {
	t.Setenv(EnvAPIToken, "")
	fallback := errors.New("pick one with astro workspace switch")
	if err := NoWorkspaceHint(fallback, HintCurrent); !errors.Is(err, fallback) {
		t.Errorf("got %v, want the fallback", err)
	}
	if err := NoWorkspace(nil); !errors.Is(err, ErrNoWorkspace) {
		t.Errorf("got %v, want ErrNoWorkspace", err)
	}
}
