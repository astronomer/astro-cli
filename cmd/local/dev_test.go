package local

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

// fakeRuntime fails every call; the engine does not exist in these tests.
type fakeRuntime struct{}

func (fakeRuntime) Start(context.Context, localrt.Plan, localrt.Callbacks) (localrt.Airflow, error) {
	return nil, localrt.ErrNotImplemented
}
func (fakeRuntime) Attach(string) (localrt.Airflow, error)    { return nil, localrt.ErrNotImplemented }
func (fakeRuntime) LogSource(string) (localrt.Airflow, error) { return nil, localrt.ErrNotImplemented }
func (fakeRuntime) ReadStatus(string) (localrt.Status, error) {
	return localrt.Status{}, localrt.ErrNotImplemented
}
func (fakeRuntime) List() ([]localrt.Status, error)       { return nil, localrt.ErrNotImplemented }
func (fakeRuntime) PruneStale() ([]localrt.Status, error) { return nil, localrt.ErrNotImplemented }

func testDeps(t *testing.T) (d Deps, stdout *bytes.Buffer) {
	t.Helper()
	stdout = &bytes.Buffer{}
	d = Deps{
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
		Stderr:     &bytes.Buffer{},
		Runtime:    fakeRuntime{},
		Checks:     stubParser{},
		WorkingDir: func() (string, error) { return t.TempDir(), nil },
		OpenURL:    func(string) error { return nil },
	}
	return d, stdout
}

func execute(t *testing.T, d Deps, args ...string) error {
	t.Helper()
	root := NewRootCmd(d)
	root.SetArgs(args)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	return root.Execute()
}

func TestDevStubNamesTheReplacement(t *testing.T) {
	cases := []struct {
		typed []string
		want  string
	}{
		{[]string{"start"}, "astro local start"},
		{[]string{"stop"}, "astro local stop"},
		{[]string{"restart"}, "astro local restart"},
		{[]string{"ps"}, "astro local status"},
		{[]string{"logs"}, "astro local logs"},
		{[]string{"run"}, "astro local run"},
		{[]string{"bash"}, "astro local shell"},
		{[]string{"parse"}, "astro local check"},
		{[]string{"kill"}, "astro local stop --clean"},
		{[]string{"pytest"}, "uv run pytest"},
		{[]string{"init"}, "astro init"},
		{[]string{"object", "import"}, "the env schema in pyproject.toml"},
		{[]string{"object", "export"}, "the env schema in pyproject.toml"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.typed, " "), func(t *testing.T) {
			d, _ := testDeps(t)
			err := execute(t, d, append([]string{"dev"}, tc.typed...)...)
			if err == nil {
				t.Fatal("astro dev must fail")
			}
			msg := err.Error()
			if !strings.Contains(msg, "Use `"+tc.want+"` instead") {
				t.Errorf("error does not name the replacement %q:\n%s", tc.want, msg)
			}
			typed := "astro dev " + strings.Join(tc.typed, " ")
			if !strings.Contains(msg, "`"+typed+"` was removed") {
				t.Errorf("error does not name the typed command %q:\n%s", typed, msg)
			}
		})
	}
}

func TestDevStubBareAndUnknown(t *testing.T) {
	d, _ := testDeps(t)
	err := execute(t, d, "dev")
	if err == nil || !strings.Contains(err.Error(), "astro dev was removed in Astro CLI v2") {
		t.Errorf("bare `astro dev` message wrong: %v", err)
	}

	err = execute(t, d, "dev", "upgrade-test")
	if err == nil {
		t.Fatal("unknown subcommand must still fail")
	}
	if !strings.Contains(err.Error(), "no direct replacement") {
		t.Errorf("unknown subcommand should say there is no direct replacement: %v", err)
	}
}

func TestDevStubV1Notice(t *testing.T) {
	writeDockerfile := func(t *testing.T, dir string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const notice = "astro v1 project (Dockerfile and .astro/)"

	t.Run("Dockerfile and .astro gets the v1 notice", func(t *testing.T) {
		dir := t.TempDir()
		writeDockerfile(t, dir)
		if err := os.Mkdir(filepath.Join(dir, ".astro"), 0o700); err != nil {
			t.Fatal(err)
		}
		d, _ := testDeps(t)
		d.WorkingDir = func() (string, error) { return dir, nil }
		err := execute(t, d, "dev")
		if err == nil || !strings.Contains(err.Error(), notice) {
			t.Errorf("v1 dir should get the v1 notice: %v", err)
		}
	})
	t.Run("Dockerfile alone gets no v1 notice", func(t *testing.T) {
		dir := t.TempDir()
		writeDockerfile(t, dir)
		d, _ := testDeps(t)
		d.WorkingDir = func() (string, error) { return dir, nil }
		err := execute(t, d, "dev")
		if err == nil {
			t.Fatal("astro dev must still fail")
		}
		if strings.Contains(err.Error(), "astro v1 project") {
			t.Errorf("a Dockerfile-only dir must not be called v1: %v", err)
		}
	})
}

func TestDevStubIgnoresFlagsWhenResolving(t *testing.T) {
	d, _ := testDeps(t)
	err := execute(t, d, "dev", "start", "--wait", "5m")
	if err == nil || !strings.Contains(err.Error(), "astro local start") {
		t.Errorf("flags must not hide the typed subcommand: %v", err)
	}
	if err != nil && !strings.Contains(err.Error(), "`astro dev start` was removed") {
		t.Errorf("flag values must not leak into the echoed command: %v", err)
	}
}

func TestDevStubJSONOutput(t *testing.T) {
	d, out := testDeps(t)
	err := execute(t, d, "dev", "ps", "--output", "json")
	if err == nil {
		t.Fatal("json mode must still fail")
	}
	var payload struct {
		Error       string `json:"error"`
		Typed       string `json:"typed_command"`
		Replacement string `json:"replacement"`
		Mapping     []struct {
			Command     string `json:"command"`
			Replacement string `json:"replacement"`
		} `json:"mapping"`
	}
	if jsonErr := json.Unmarshal(out.Bytes(), &payload); jsonErr != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", jsonErr, out.String())
	}
	if payload.Replacement != "astro local status" {
		t.Errorf("replacement = %q, want astro local status", payload.Replacement)
	}
	if payload.Typed != "astro dev ps" {
		t.Errorf("typed_command = %q", payload.Typed)
	}
	if len(payload.Mapping) == 0 {
		t.Errorf("payload missing mapping: %+v", payload)
	}
}

func TestDevStubTextMatchesJSONData(t *testing.T) {
	p := buildDevRemoved("ps", false)
	text := renderDevRemoved(p)
	if !strings.Contains(text, p.Error) || !strings.Contains(text, p.Replacement) {
		t.Errorf("text rendering dropped payload data:\n%s", text)
	}
}
