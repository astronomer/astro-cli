package local

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/pkg/envschema"
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

func TestWarnStandaloneOmissionsPackages(t *testing.T) {
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
			warnStandaloneOmissions(Renderer{Format: FormatText, Out: text}, tc.plan)
			if tc.wantWarn {
				if !strings.Contains(text.String(), "warning: this project declares OS packages") {
					t.Errorf("text output missing the warning: %q", text.String())
				}
			} else if text.Len() != 0 {
				t.Errorf("expected no warning, got %q", text.String())
			}

			// JSON mode: a single warning event line, or nothing.
			jsonOut := &bytes.Buffer{}
			warnStandaloneOmissions(Renderer{Format: FormatJSON, Out: jsonOut}, tc.plan)
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

// Standalone says so when it is ignoring a declared Dockerfile.
//
// This was the larger of the two omissions and the one with no warning. OS
// packages are a line in the manifest; a declared Dockerfile IS the build. So
// `astro local start` on a project whose Dockerfile does `RUN apt-get install -y
// unixodbc-dev` provisioned an environment with none of it and said nothing —
// and standalone is the DEFAULT, so that is what a user gets from the shortest
// command. rt.Plan's doc records that standalone ignores the field; nothing said
// it to the person it happens to.
func TestWarnStandaloneDockerfile(t *testing.T) {
	const declared = "docker/Dockerfile"
	cases := []struct {
		name     string
		plan     localrt.Plan
		wantWarn bool
	}{
		{"standalone with a declared Dockerfile warns", localrt.Plan{Dockerfile: declared}, true},
		{"explicit standalone warns", localrt.Plan{Mode: localrt.ModeStandalone, Dockerfile: declared}, true},
		{"docker mode is silent, because it builds the file", localrt.Plan{Mode: localrt.ModeDocker, Dockerfile: declared}, false},
		{"no declaration is silent", localrt.Plan{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := &bytes.Buffer{}
			warnStandaloneOmissions(Renderer{Format: FormatText, Out: text}, tc.plan)
			if !tc.wantWarn {
				if text.Len() != 0 {
					t.Errorf("expected no warning, got %q", text.String())
				}
				return
			}
			got := text.String()
			if !strings.Contains(got, "declares its own Dockerfile") {
				t.Errorf("output missing the Dockerfile warning: %q", got)
			}
			// The path, because "a Dockerfile" is not actionable when the
			// manifest may name one in a subdirectory.
			if !strings.Contains(got, declared) {
				t.Errorf("output does not name the declared path: %q", got)
			}
			if !strings.Contains(got, "--docker") {
				t.Errorf("output does not say what to do about it: %q", got)
			}
		})
	}
}

// A project declaring both gets both warnings, and the Dockerfile leads: it is
// the one that changes what the user should do.
func TestWarnStandaloneReportsBothOmissions(t *testing.T) {
	text := &bytes.Buffer{}
	warnStandaloneOmissions(Renderer{Format: FormatText, Out: text}, localrt.Plan{
		Dockerfile: "Dockerfile",
		Packages:   []string{"libpq-dev"},
	})
	got := text.String()
	dockerfileAt := strings.Index(got, "declares its own Dockerfile")
	packagesAt := strings.Index(got, "declares OS packages")
	if dockerfileAt < 0 || packagesAt < 0 {
		t.Fatalf("expected both warnings, got %q", got)
	}
	if dockerfileAt > packagesAt {
		t.Errorf("the Dockerfile warning should lead; got %q", got)
	}
}

// An omission kind this build has no wording for still reaches the user.
func TestOmissionTextUnknownKind(t *testing.T) {
	got := omissionText(localrt.Omission{Kind: "something-new"})
	if !strings.Contains(got, "something-new") || !strings.Contains(got, "standalone") {
		t.Errorf("unknown kind text does not name the kind: %q", got)
	}
}

// The omission warnings, byte for byte, in both output formats. Scripts and
// Astro Desktop's logs match on this text, so a rewording is a deliberate change
// to this test rather than a side effect.
func TestWarnStandaloneOmissionsExactText(t *testing.T) {
	plan := localrt.Plan{
		Dockerfile: "docker/Dockerfile",
		Packages:   []string{"libpq-dev"},
	}
	const dockerfileText = "this project declares its own Dockerfile (docker/Dockerfile); standalone mode builds no image, so nothing that file installs or copies is applied. Run in Docker mode (--docker) to build it"
	const packagesText = "this project declares OS packages; standalone mode cannot install them, run in Docker mode (--docker) or install them yourself"

	text := &bytes.Buffer{}
	warnStandaloneOmissions(Renderer{Format: FormatText, Out: text}, plan)
	if want := "warning: " + dockerfileText + "\nwarning: " + packagesText + "\n"; text.String() != want {
		t.Errorf("text output:\n got %q\nwant %q", text.String(), want)
	}

	jsonOut := &bytes.Buffer{}
	warnStandaloneOmissions(Renderer{Format: FormatJSON, Out: jsonOut}, plan)
	lines := strings.Split(strings.TrimSpace(jsonOut.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want two JSON warning lines, got %q", jsonOut.String())
	}
	for i, want := range []string{dockerfileText, packagesText} {
		var e event
		if err := json.Unmarshal([]byte(lines[i]), &e); err != nil {
			t.Fatalf("line %d is not JSON: %v: %q", i, err, lines[i])
		}
		if (e != event{Event: "warning", Text: want}) {
			t.Errorf("line %d: got %+v, want a warning with text %q", i, e, want)
		}
	}
}

// envWarningCase is one warning-reporting expectation, shared by the text and
// JSON tests below: one table, two renderings.
type envWarningCase struct {
	name     string
	warnings []envschema.Violation
	wantText []string
}

func envWarningCases() []envWarningCase {
	return []envWarningCase{
		{
			name:     "nothing to report is silent",
			warnings: nil,
		},
		{
			name: "an env var names its section, key and reason",
			warnings: []envschema.Violation{{
				Kind:    envschema.ViolationWrongType,
				Section: envschema.SectionEnvVar,
				Key:     "PORT",
				Reason:  `expected a port between 1 and 65535, got "99999"`,
			}},
			wantText: []string{`warning: env var PORT: expected a port between 1 and 65535, got "99999"`},
		},
		{
			// The wire value is snake_case; a person reads this.
			name: "an Airflow variable is named the way a person would",
			warnings: []envschema.Violation{{
				Kind:    envschema.ViolationWrongType,
				Section: envschema.SectionAirflowVariable,
				Key:     "mode",
				Reason:  `expected one of "a", "b", got "c"`,
			}},
			wantText: []string{`warning: Airflow variable mode:`},
		},
		{
			name: "a connection of the wrong kind",
			warnings: []envschema.Violation{{
				Kind:    envschema.ViolationWrongType,
				Section: envschema.SectionConnection,
				Key:     "warehouse",
				Reason:  `declared conn_type "snowflake" but resolved to "postgres"`,
			}},
			wantText: []string{`warning: connection warehouse: declared conn_type "snowflake" but resolved to "postgres"`},
		},
		{
			// One line each: a project with several is a list, not a summary.
			name: "every finding gets its own line",
			warnings: []envschema.Violation{
				{Kind: envschema.ViolationWrongType, Section: envschema.SectionEnvVar, Key: "A", Reason: "one"},
				{Kind: envschema.ViolationWrongType, Section: envschema.SectionEnvVar, Key: "B", Reason: "two"},
			},
			wantText: []string{"warning: env var A: one", "warning: env var B: two"},
		},
	}
}

// warnEnvValues reports value-level findings without refusing the start.
func TestWarnEnvValuesText(t *testing.T) {
	for _, tc := range envWarningCases() {
		t.Run(tc.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			warnEnvValues(Renderer{Format: FormatText, Out: out}, tc.warnings)
			if len(tc.wantText) == 0 {
				if out.Len() != 0 {
					t.Fatalf("expected no output, got %q", out.String())
				}
				return
			}
			for _, want := range tc.wantText {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output missing %q, got %q", want, out.String())
				}
			}
			if got := strings.Count(out.String(), "warning:"); got != len(tc.warnings) {
				t.Errorf("got %d warning lines, want %d: %q", got, len(tc.warnings), out.String())
			}
		})
	}
}

// One warning event per finding, each parseable on its own line so a consumer
// can stream them.
func TestWarnEnvValuesJSON(t *testing.T) {
	for _, tc := range envWarningCases() {
		t.Run(tc.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			warnEnvValues(Renderer{Format: FormatJSON, Out: out}, tc.warnings)
			if len(tc.warnings) == 0 {
				if out.Len() != 0 {
					t.Fatalf("expected no output, got %q", out.String())
				}
				return
			}
			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			if len(lines) != len(tc.warnings) {
				t.Fatalf("got %d JSON lines, want %d: %q", len(lines), len(tc.warnings), out.String())
			}
			for i, line := range lines {
				assertWarningEvent(t, line, tc.warnings[i])
			}
		})
	}
}

// assertWarningEvent checks one JSON warning line against the violation it came
// from, so the structured-field expectations live in one place.
//
// The section, key and reason must survive as their own fields, so a consumer
// does not have to regex sectionLabel's human strings back out of Text.
func assertWarningEvent(t *testing.T, line string, want envschema.Violation) {
	t.Helper()
	var e event
	if err := json.Unmarshal([]byte(line), &e); err != nil {
		t.Fatalf("warning is not valid JSON: %v: %q", err, line)
	}
	if e.Event != "warning" {
		t.Errorf("event = %q, want %q", e.Event, "warning")
	}
	if e.Section != string(want.Section) {
		t.Errorf("section = %q, want %q", e.Section, want.Section)
	}
	if e.Key != want.Key {
		t.Errorf("key = %q, want %q", e.Key, want.Key)
	}
	if e.Reason != want.Reason {
		t.Errorf("reason = %q, want %q", e.Reason, want.Reason)
	}
}
