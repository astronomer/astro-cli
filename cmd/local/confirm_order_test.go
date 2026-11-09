package local

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/pkg/input"
)

// confirmLines drives confirm with stdin and reports what it returned and
// wrote to stderr, at a terminal or not.
func confirmLines(t *testing.T, interactive bool, stdin string) (string, error) {
	t.Helper()
	d, _ := testDeps(t)
	d.Stdin = strings.NewReader(stdin)
	d.Interactive = func() bool { return interactive }
	c := &cli{d: d}
	err := c.confirm("Wipe it?")
	return stderrOf(d), err
}

// Every way a confirmation ends leaves stderr with whole lines: the question
// is written only where someone reads it, a line that was opened is ended
// before the error, and nothing writes a blank line of its own.
func TestConfirmLeavesWholeLines(t *testing.T) {
	const q = "Wipe it? [y/N] "
	for name, tc := range map[string]struct {
		interactive bool
		stdin       string
		stderr      string
		refused     bool  // the input_required refusal naming --yes
		err         error // otherwise; nil for a yes
	}{
		"terminal, yes":               {true, "y\n", q, false, nil},
		"terminal, no":                {true, "n\n", q, false, input.ErrAborted},
		"terminal, input ended":       {true, "", q + "\n", true, nil},
		"terminal, yes then ended":    {true, "y", q + "\n", false, nil},
		"piped, yes":                  {false, "y\n", q + "y\n", false, nil},
		"piped, no":                   {false, "n\n", q + "n\n", false, input.ErrAborted},
		"piped, yes with no newline":  {false, "y", q + "y\n", false, nil},
		"piped nothing":               {false, "", "", true, nil},
		"piped, an empty line (a no)": {false, "\n", "Wipe it? [y/N]\n", false, input.ErrAborted},
	} {
		t.Run(name, func(t *testing.T) {
			stderr, err := confirmLines(t, tc.interactive, tc.stdin)
			if stderr != tc.stderr {
				t.Errorf("stderr = %q, want %q", stderr, tc.stderr)
			}
			switch {
			case tc.refused:
				if !input.IsRequired(err) || !strings.Contains(err.Error(), "pass --yes") {
					t.Errorf("err = %v, want the refusal naming --yes", err)
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

// Ctrl-C ends a question at once, with ErrInterrupted, and ends its line only
// where one was written.
func TestConfirmInterruptedEndsOnlyAnOpenLine(t *testing.T) {
	for name, interactive := range map[string]bool{"terminal": true, "piped": false} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			t.Cleanup(input.SetInterrupt(ctx))
			stdin := blockingStdin{cancel: cancel, release: make(chan struct{})}
			t.Cleanup(func() { close(stdin.release) })

			d, _ := testDeps(t)
			d.Stdin = stdin
			d.Stderr = &bytes.Buffer{}
			d.Interactive = func() bool { return interactive }
			err := (&cli{d: d}).confirm("Wipe it?")
			if !errors.Is(err, input.ErrInterrupted) {
				t.Fatalf("err = %v, want ErrInterrupted", err)
			}
			want := ""
			if interactive {
				want = "Wipe it? [y/N] \n"
			}
			if got := stderrOf(d); got != want {
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
}

// With no terminal and no answer, the refusal is all stderr gets: no question
// left open with the error run onto the end of it.
func TestResetWithNoTerminalRefusesWithoutAsking(t *testing.T) {
	d, rt, _ := resetDeps(t)
	err := execute(t, d, "local", "reset")
	if err == nil || !strings.HasPrefix(err.Error(), `confirmation needed for "Wipe the local Airflow state`) ||
		!strings.HasSuffix(err.Error(), "but stdin is not interactive; pass --yes to proceed") {
		t.Fatalf("err = %v, want the refusal naming the question and --yes", err)
	}
	if kind := ProblemKinds.Of(err); kind != cliout.KindInputRequired {
		t.Errorf("kind %q, want %q", kind, cliout.KindInputRequired)
	}
	if stderr := stderrOf(d); stderr != "" {
		t.Errorf("stderr = %q, want nothing before the error", stderr)
	}
	if rt.called != "" {
		t.Error("reset ran without consent")
	}
}

// The question names the project it would wipe. At a terminal it is asked
// before the answer is read; from a pipe it is recorded with the answer.
func TestResetQuestionNamesTheProject(t *testing.T) {
	for name, interactive := range map[string]bool{"terminal": true, "piped": false} {
		t.Run(name, func(t *testing.T) {
			d, rt, _ := resetDeps(t)
			dir, _ := d.WorkingDir()
			d.Stdin = strings.NewReader("y\n")
			d.Interactive = func() bool { return interactive }
			if err := execute(t, d, "local", "reset"); err != nil {
				t.Fatalf("reset answered yes: %v", err)
			}
			stderr := stderrOf(d)
			want := "Wipe the local Airflow state (database, logs, environment) of " + dir + "? [y/N] "
			if !strings.Contains(stderr, want) {
				t.Errorf("stderr = %q, want the question naming %s", stderr, dir)
			}
			if !interactive && !strings.Contains(stderr, want+"y\n") {
				t.Errorf("a piped answer should be recorded with its question: %q", stderr)
			}
			if rt.called != dir {
				t.Errorf("reset ran on %q, want %q", rt.called, dir)
			}
		})
	}
}
