package local

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

func TestParseFormat(t *testing.T) {
	for _, ok := range []string{"text", "json"} {
		if _, err := ParseFormat(ok); err != nil {
			t.Errorf("ParseFormat(%q) = %v", ok, err)
		}
	}
	if _, err := ParseFormat("yaml"); err == nil {
		t.Error("ParseFormat(yaml) should fail")
	}
}

func TestEmitJSONIsOneLine(t *testing.T) {
	out := &bytes.Buffer{}
	r := Renderer{Format: FormatJSON, Out: out}
	st := localrt.Status{ProjectPath: "/p", State: localrt.StateRunning, Port: 8080}
	if err := r.Emit(st, func(io.Writer) error { t.Fatal("text renderer must not run in json mode"); return nil }); err != nil {
		t.Fatal(err)
	}
	line := out.String()
	if strings.Count(line, "\n") != 1 || !strings.HasSuffix(line, "\n") {
		t.Errorf("json emit is not exactly one line: %q", line)
	}
	var got localrt.Status
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if got != st {
		t.Errorf("round trip changed the value: %+v != %+v", got, st)
	}
}

func TestEmitStreamsNDJSON(t *testing.T) {
	out := &bytes.Buffer{}
	r := Renderer{Format: FormatJSON, Out: out}
	for i := range 3 {
		e := event{Event: "log", Component: "scheduler", Text: fmt.Sprintf("line %d", i)}
		if err := r.Emit(e, nil); err != nil {
			t.Fatal(err)
		}
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 NDJSON lines, got %d: %q", len(lines), out.String())
	}
	for _, l := range lines {
		var e event
		if err := json.Unmarshal([]byte(l), &e); err != nil {
			t.Errorf("line is not standalone JSON: %v: %q", err, l)
		}
	}
}

func TestEmitTextRendersSameValue(t *testing.T) {
	out := &bytes.Buffer{}
	r := Renderer{Format: FormatText, Out: out}
	l := localrt.LogLine{Component: "scheduler", Time: time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC), Text: "heartbeat"}
	if err := r.Emit(logEvent(l), func(w io.Writer) error { return renderLogLine(w, l) }); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"scheduler", "heartbeat", "2026-07-21T12:00:00Z"} {
		if !strings.Contains(got, want) {
			t.Errorf("text output %q is missing %q from the shared data", got, want)
		}
	}
}

func TestWarnStandalonePackages(t *testing.T) {
	packages := []string{"libpq-dev"}
	cases := []struct {
		name     string
		plan     localrt.Plan
		wantWarn bool
	}{
		{"standalone with packages warns", localrt.Plan{Packages: packages}, true},
		{"explicit standalone with packages warns", localrt.Plan{Mode: localrt.ModeStandalone, Packages: packages}, true},
		{"docker with packages is silent", localrt.Plan{Mode: localrt.ModeDocker, Packages: packages}, false},
		{"standalone without packages is silent", localrt.Plan{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Text mode: a warning line, or nothing.
			text := &bytes.Buffer{}
			warnStandalonePackages(Renderer{Format: FormatText, Out: text}, tc.plan)
			if tc.wantWarn {
				if !strings.Contains(text.String(), "warning: this project declares OS packages") {
					t.Errorf("text output missing the warning: %q", text.String())
				}
			} else if text.Len() != 0 {
				t.Errorf("expected no warning, got %q", text.String())
			}

			// JSON mode: a single warning event line, or nothing.
			jsonOut := &bytes.Buffer{}
			warnStandalonePackages(Renderer{Format: FormatJSON, Out: jsonOut}, tc.plan)
			if !tc.wantWarn {
				if jsonOut.Len() != 0 {
					t.Errorf("expected no JSON warning, got %q", jsonOut.String())
				}
				return
			}
			var e event
			if err := json.Unmarshal(bytes.TrimSpace(jsonOut.Bytes()), &e); err != nil {
				t.Fatalf("warning is not valid JSON: %v: %q", err, jsonOut.String())
			}
			if e.Event != "warning" || !strings.Contains(e.Text, "standalone mode cannot install them") {
				t.Errorf("unexpected warning event: %+v", e)
			}
		})
	}
}

func TestCommandsRejectUnknownOutputFormat(t *testing.T) {
	d, _ := testDeps(t)
	err := execute(t, d, "local", "status", "--output", "yaml")
	if err == nil || !strings.Contains(err.Error(), "unknown output format") {
		t.Errorf("want unknown output format error, got %v", err)
	}
}

func TestFailingCommandEmitsJSONErrorObject(t *testing.T) {
	// The fake runtime fails every call, so each of these commands returns an
	// error. Under --output json the failure must be one JSON error object on
	// stdout, not a plaintext line on stderr.
	cases := [][]string{
		{"local", "status", "-o", "json"},
		{"local", "stop", "-o", "json"},
		{"local", "logs", "-o", "json"},
		{"stop", "-o", "json"}, // root alias
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			d, stdout := testDeps(t)
			if err := execute(t, d, args...); err == nil {
				t.Fatalf("%v should fail", args)
			}
			line := strings.TrimSpace(stdout.String())
			if strings.Count(line, "\n") != 0 {
				t.Fatalf("want one JSON object, got %q", stdout.String())
			}
			var obj struct {
				Error string `json:"error"`
				Code  int    `json:"code"`
			}
			if err := json.Unmarshal([]byte(line), &obj); err != nil {
				t.Fatalf("stdout is not a JSON error object: %q (%v)", stdout.String(), err)
			}
			if obj.Error == "" {
				t.Errorf("error field is empty: %q", line)
			}
			if obj.Code == 0 {
				t.Errorf("code field should be non-zero: %q", line)
			}
		})
	}
}

func TestUnknownLocalSubcommandFails(t *testing.T) {
	d, _ := testDeps(t)
	if err := execute(t, d, "local", "bogus"); err == nil {
		t.Fatal("`astro local bogus` must fail, not print help and exit 0")
	}
	// A bare `astro local` prints help and succeeds.
	d, _ = testDeps(t)
	if err := execute(t, d, "local"); err != nil {
		t.Fatalf("bare `astro local` should succeed: %v", err)
	}
}

func TestStatusJSONIsLowercaseAndOmitsZeroFields(t *testing.T) {
	// A stopped status carries no pid, port, or start time; the json shape must
	// use lowercase keys and drop those zero fields (no PascalCase, no leaked
	// 0001-01-01 timestamp), matching the rest of the v2 surface.
	out := &bytes.Buffer{}
	r := Renderer{Format: FormatJSON, Out: out}
	st := localrt.Status{ProjectPath: "/p", State: localrt.StateStopped}
	if err := r.Emit(st, nil); err != nil {
		t.Fatal(err)
	}
	var keys map[string]any
	if err := json.Unmarshal(out.Bytes(), &keys); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	for _, gone := range []string{"StartedAt", "startedAt", "PID", "pid", "Port", "port", "Mode", "mode"} {
		if _, ok := keys[gone]; ok {
			t.Errorf("stopped status should omit %q: %v", gone, keys)
		}
	}
	for _, want := range []string{"projectPath", "state"} {
		if _, ok := keys[want]; !ok {
			t.Errorf("status json missing %q: %v", want, keys)
		}
	}
	if strings.Contains(out.String(), "0001-01-01") {
		t.Errorf("zero StartedAt leaked into json: %q", out.String())
	}
}

func TestConfirmEOFIsAnErrorNotANo(t *testing.T) {
	d, _ := testDeps(t)
	d.Stdin = strings.NewReader("") // closed stdin
	c := &cli{d: d}
	err := c.confirm("wipe?")
	if err == nil {
		t.Fatal("EOF must be an error")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("EOF error should point at --yes: %v", err)
	}

	d.Stdin = strings.NewReader("y\n")
	c = &cli{d: d}
	if err := c.confirm("wipe?"); err != nil {
		t.Errorf("explicit yes should pass: %v", err)
	}

	d.Stdin = strings.NewReader("n\n")
	c = &cli{d: d}
	if err := c.confirm("wipe?"); err == nil {
		t.Error("explicit no should abort")
	}
}
