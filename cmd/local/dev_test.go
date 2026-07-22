package local

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

// fakeRuntime fails every call; the engine does not exist in these tests.
type fakeRuntime struct{}

func (fakeRuntime) Start(context.Context, localrt.Plan, localrt.Callbacks) (localrt.Airflow, error) {
	return nil, localrt.ErrNotImplemented
}
func (fakeRuntime) Attach(string) (localrt.Airflow, error) { return nil, localrt.ErrNotImplemented }
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
		{[]string{"object", "import"}, "astro local env schema"},
		{[]string{"object", "export"}, "astro local env schema"},
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
			if !strings.Contains(msg, devMappingDoc) {
				t.Errorf("error does not link the full mapping:\n%s", msg)
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
	if !strings.Contains(err.Error(), devMappingDoc) {
		t.Errorf("unknown subcommand should link the mapping: %v", err)
	}
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
		Doc string `json:"doc"`
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
	if len(payload.Mapping) == 0 || payload.Doc == "" {
		t.Errorf("payload missing mapping or doc: %+v", payload)
	}
}

func TestDevStubTextMatchesJSONData(t *testing.T) {
	p := buildDevRemoved("ps", false)
	text := renderDevRemoved(p)
	if !strings.Contains(text, p.Error) || !strings.Contains(text, p.Replacement) || !strings.Contains(text, p.Doc) {
		t.Errorf("text rendering dropped payload data:\n%s", text)
	}
}
