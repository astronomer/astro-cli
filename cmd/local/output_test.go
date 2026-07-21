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

func TestCommandsRejectUnknownOutputFormat(t *testing.T) {
	d, _ := testDeps(t)
	err := execute(t, d, "local", "status", "--output", "yaml")
	if err == nil || !strings.Contains(err.Error(), "unknown output format") {
		t.Errorf("want unknown output format error, got %v", err)
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
