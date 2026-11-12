package local

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/pkg/input"
)

// confirmLines drives confirm with stdin and reports what it returned and
// wrote to stderr.
func confirmLines(t *testing.T, stdin string) (string, error) {
	t.Helper()
	d, _ := testDeps(t)
	d.Stdin = strings.NewReader(stdin)
	c := &cli{d: d}
	err := c.confirm("Wipe it?")
	return stderrOf(d), err
}

// Every way a confirmation ends leaves stderr with whole lines. The question
// is written first, a terminal echoes the answer itself, and input that ended
// (Ctrl-D) has its line ended before the refusal. A run with no terminal is
// refused before this is reached (pkg/input's tests hold that rule).
func TestConfirmLeavesWholeLines(t *testing.T) {
	const (
		q       = "Wipe it? [y/N] "
		refused = "confirmation needed but input ended with no answer; pass --yes to proceed"
	)
	for name, tc := range map[string]struct {
		stdin   string
		stderr  string
		refusal string // the input_required refusal naming --yes
		err     error  // otherwise; nil for a yes
	}{
		"yes":                           {"y\n", q, "", nil},
		"no":                            {"n\n", q, "", input.ErrAborted},
		"input ended":                   {"", q + "\n", refused, nil},
		"yes then ended":                {"y", q + "\n", "", nil},
		"an empty line (a no)":          {"\n", q, "", input.ErrAborted},
		"only escapes and no newline":   {"\x1b[0m", q + "\n", refused, nil},
		"a joiner keeps y-es from yes":  {"y\u200des\n", q, "", input.ErrAborted},
		"a styled yes is the yes shown": {"y\x1b[0m\x07\n", q, "", nil},
	} {
		t.Run(name, func(t *testing.T) {
			stderr, err := confirmLines(t, tc.stdin)
			if stderr != tc.stderr {
				t.Errorf("stderr = %q, want %q", stderr, tc.stderr)
			}
			switch {
			case tc.refusal != "":
				if !input.IsRequired(err) || err.Error() != tc.refusal {
					t.Errorf("err = %v, want the refusal %q", err, tc.refusal)
				}
			case tc.err == nil && err != nil:
				t.Errorf("err = %v, want a yes", err)
			case tc.err != nil && !errors.Is(err, tc.err):
				t.Errorf("err = %v, want %v", err, tc.err)
			}
		})
	}
}

// blockingStdin is a stdin nobody answers: its read interrupts the run, as
// Ctrl-C would while the question waits, then blocks until the test ends.
type blockingStdin struct {
	cancel  context.CancelFunc
	release chan struct{}
}

func (b blockingStdin) Read([]byte) (int, error) {
	b.cancel()
	<-b.release
	return 0, context.Canceled
}

// Ctrl-C ends a question at once, with ErrInterrupted, and ends the
// question's line first, a terminal on stdin or not, so the error does not
// run onto it ("^CError: interrupted").
func TestConfirmInterruptedEndsTheQuestionsLine(t *testing.T) {
	for name := range map[string]bool{"terminal": true} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			t.Cleanup(input.SetInterrupt(ctx))
			stdin := blockingStdin{cancel: cancel, release: make(chan struct{})}
			t.Cleanup(func() { close(stdin.release) })

			d, _ := testDeps(t)
			d.Stdin = stdin
			d.Stderr = &bytes.Buffer{}
			err := (&cli{d: d}).confirm("Wipe it?")
			if !errors.Is(err, input.ErrInterrupted) {
				t.Fatalf("err = %v, want ErrInterrupted", err)
			}
			if got, want := stderrOf(d), "Wipe it? [y/N] \n"; got != want {
				t.Errorf("stderr = %q, want %q", got, want)
			}
		})
	}
}

// A confirmation asks about a target the command has already found. Every
// failure that needs no consent (no project here, no home for the state) comes
// first, so the person is never asked to agree to something that was never
// going to happen, and the refusal is the one any other command gives.
func TestConfirmationsComeAfterTheProjectIsFound(t *testing.T) {
	for name, args := range map[string][]string{
		"local reset":           {"local", "reset"},
		"local af runs delete":  {"local", "af", "runs", "delete", "orders_etl", "run_1"},
		"local af runs clear":   {"local", "af", "runs", "clear", "orders_etl", "run_1"},
		"local af tasks clear":  {"local", "af", "tasks", "clear", "orders_etl", "run_1", "extract"},
		"link remove, no NAME":  {"link", "remove"},
		"link default, no NAME": {"link", "default"},
	} {
		t.Run(name, func(t *testing.T) {
			d, _ := testDeps(t) // a working dir with no pyproject.toml
			answer := strings.NewReader("y\n")
			d.Stdin = answer
			err := execute(t, d, args...)
			if err == nil {
				t.Fatal("a directory with no project must fail")
			}
			if kind := ProblemKinds.Of(err); kind != KindNoProject {
				t.Errorf("kind %q, want %q: %v", kind, KindNoProject, err)
			}
			if !strings.Contains(err.Error(), "no Astro project found") {
				t.Errorf("err = %v, want the no-project error", err)
			}
			if stderr := stderrOf(d); strings.Contains(stderr, "[y/N]") {
				t.Errorf("asked before finding the project: %q", stderr)
			}
			if answer.Len() == 0 {
				t.Error("read an answer before finding the project")
			}
		})
	}
}

// Without a home, reset could only fail after the wipe was agreed to, so it
// fails before asking.
func TestResetWithNoHomeFailsBeforeAsking(t *testing.T) {
	d, rt, _ := resetDeps(t)
	d.HomeErr = errors.New("no home directory")
	err := execute(t, d, "local", "reset")
	if err == nil || err.Error() != "no home directory" {
		t.Fatalf("err = %v, want the home error", err)
	}
	if stderr := stderrOf(d); strings.Contains(stderr, "[y/N]") {
		t.Errorf("asked with no home: %q", stderr)
	}
	if rt.called != "" {
		t.Error("reset reached the runtime")
	}

	// A missing home is a failure, not a question this run could not ask, so
	// under --output json it is the home error and not input_required.
	d, rt, out := resetDeps(t)
	d.HomeErr = errors.New("no home directory")
	err = execute(t, d, "local", "reset", "-o", "json")
	if err == nil || ProblemKinds.Of(err) == cliout.KindInputRequired {
		t.Fatalf("err = %v (kind %q), want the home error, not input_required", err, ProblemKinds.Of(err))
	}
	if strings.Contains(out.String(), "input_required") || !strings.Contains(out.String(), "no home directory") {
		t.Errorf("json = %q, want the home error", out.String())
	}
	if rt.called != "" {
		t.Error("reset reached the runtime")
	}
}

// At a terminal with no answer (Ctrl-D), reset ends the question's line and
// refuses naming --yes: the error starts on a line of its own rather than
// running onto the question.
func TestResetWithNoAnswerAsksThenRefuses(t *testing.T) {
	d, rt, _ := resetDeps(t)
	dir, _ := d.WorkingDir()
	err := execute(t, d, "local", "reset")
	if err == nil || err.Error() != "confirmation needed but input ended with no answer; pass --yes to proceed" {
		t.Fatalf("err = %v, want the refusal naming --yes", err)
	}
	if kind := ProblemKinds.Of(err); kind != cliout.KindInputRequired {
		t.Errorf("kind %q, want %q", kind, cliout.KindInputRequired)
	}
	want := "Wipe the local Airflow state (database, logs, environment) of " + dir + "? [y/N] \n"
	if stderr := stderrOf(d); stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	if rt.called != "" {
		t.Error("reset ran without consent")
	}
}

// The question names the project it would wipe. At a terminal it is asked
// before the answer is read.
func TestResetQuestionNamesTheProject(t *testing.T) {
	for name := range map[string]bool{"terminal": true} {
		t.Run(name, func(t *testing.T) {
			d, rt, _ := resetDeps(t)
			dir, _ := d.WorkingDir()
			d.Stdin = strings.NewReader("y\n")
			if err := execute(t, d, "local", "reset"); err != nil {
				t.Fatalf("reset answered yes: %v", err)
			}
			stderr := stderrOf(d)
			want := "Wipe the local Airflow state (database, logs, environment) of " + dir + "? [y/N] "
			if !strings.Contains(stderr, want) {
				t.Errorf("stderr = %q, want the question naming %s", stderr, dir)
			}
			if rt.called != dir {
				t.Errorf("reset ran on %q, want %q", rt.called, dir)
			}
		})
	}
}

// A project directory can be named with an escape sequence. The question
// names the path, so the path is cleaned where the question is built: nothing
// reaches the terminal that moves the cursor or retitles the window. The
// wipe itself still runs on the real path.
func TestResetQuestionCleansTheProjectPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows file name cannot hold an escape")
	}
	d, rt, _ := resetDeps(t)
	name := "proj\x1b]0;pwned\x07\x1b[2J\x1bPpayload\x1b\\" // a window title, a clear screen, a device control string
	dir := t.TempDir() + string(filepath.Separator) + name
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(validManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return dir, nil }
	d.Stdin = strings.NewReader("y\n")
	if err := execute(t, d, "local", "reset"); err != nil {
		t.Fatalf("reset answered yes: %v", err)
	}
	stderr := stderrOf(d)
	if strings.ContainsAny(stderr, "\x1b\x07") {
		t.Errorf("an escape from the directory name reached the terminal: %q", stderr)
	}
	if want := "/proj? [y/N] "; !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want the question to end the path with %q", stderr, want)
	}
	if rt.called != dir {
		t.Errorf("reset ran on %q, want the real path %q", rt.called, dir)
	}
}

// Two sibling projects whose names differ only in their spacing are named
// apart in the question, so a person is never asked about one while the wipe
// runs on the other.
func TestResetQuestionKeepsThePathsSpacing(t *testing.T) {
	d, rt, _ := resetDeps(t)
	dir := t.TempDir() + string(filepath.Separator) + "my  proj"
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(validManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return dir, nil }
	d.Stdin = strings.NewReader("y\n")
	if err := execute(t, d, "local", "reset"); err != nil {
		t.Fatalf("reset answered yes: %v", err)
	}
	if want := "of " + dir + "? [y/N] "; !strings.Contains(stderrOf(d), want) {
		t.Errorf("stderr = %q, want the path as it is, in %q", stderrOf(d), want)
	}
	if rt.called != dir {
		t.Errorf("reset ran on %q, want %q", rt.called, dir)
	}
}

// partialThenFails is a stdin whose read returns part of a line and then
// fails, as a broken pipe or a console that went away does.
type partialThenFails struct{ done bool }

func (p *partialThenFails) Read(b []byte) (int, error) {
	if p.done {
		return 0, errors.New("read failed")
	}
	p.done = true
	return copy(b, "y"), nil
}

// A read that fails after some text is not a yes: only a line that ended at
// the end of input counts as an unterminated answer.
func TestConfirmReadFailureWithPartialTextIsNotAYes(t *testing.T) {
	for name := range map[string]bool{"terminal": true} {
		t.Run(name, func(t *testing.T) {
			d, _ := testDeps(t)
			d.Stdin = &partialThenFails{}
			err := (&cli{d: d}).confirm("Wipe it?")
			if err == nil || !strings.Contains(err.Error(), "read failed") {
				t.Fatalf("err = %v, want the read failure, not a yes", err)
			}
			if errors.Is(err, input.ErrAborted) || input.IsRequired(err) {
				t.Errorf("err = %v, want the read error as it came", err)
			}
		})
	}
}
