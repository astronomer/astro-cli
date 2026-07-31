package instances

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// helperEnv tells the helper process below which shape of auth helper to be.
// The exec method inherits the environment whole, so this is also the test
// that it does.
const helperEnv = "ASTRO_TEST_EXEC_HELPER"

// TestExecHelperProcess is not a test. It is the program the exec tests run:
// the test binary re-executes itself with this one test selected, which is how
// a Go test gets a real subprocess without shipping a fixture binary. It exits
// before the testing package can print anything, so its stdout is only ever
// what the helper meant to say.
func TestExecHelperProcess(t *testing.T) {
	behavior := os.Getenv(helperEnv)
	if behavior == "" {
		t.Skip("not the helper process")
	}
	switch behavior {
	case "token":
		fmt.Fprintln(os.Stdout, "  s3cr3t-token  ")
	case "empty":
	case "whitespace":
		fmt.Fprintln(os.Stdout, "   ")
	case "multiline":
		fmt.Fprintln(os.Stdout, "s3cr3t-token")
		fmt.Fprintln(os.Stdout, "warning: your session expires soon")
	case "fail":
		fmt.Fprintln(os.Stderr, "acme-airflow-token: no SSO session")
		os.Exit(3) //nolint:forbidigo // the helper is a program, not a test: it exits with the code the case is about
	case "hang":
		time.Sleep(time.Minute)
	case "args":
		fmt.Fprintln(os.Stdout, strings.Join(flag.Args(), "|"))
	}
	os.Exit(0) //nolint:forbidigo // exiting here is what keeps the testing package from printing over the helper's stdout
}

// helperArgv is the argv that runs this test binary as the auth helper. The
// command is built here rather than parsed out of a manifest because it holds
// a path only this process knows.
func helperArgv(extra ...string) []string {
	argv := []string{os.Args[0], "-test.run=^TestExecHelperProcess$"}
	if len(extra) == 0 {
		return argv
	}
	// The helper is a test binary, so its own flags come first and everything
	// after -- is the arguments under test.
	return append(append(argv, "--"), extra...)
}

func execLink(argv []string) manifest.Link {
	return manifest.Link{URL: "https://airflow.acme.internal", Auth: manifest.Auth{Method: manifest.AuthExec, Command: argv}}
}

// execInstance builds an exec-method instance whose command is the helper.
func execInstance(t *testing.T, behavior string, extra ...string) Instance {
	t.Helper()
	t.Setenv(helperEnv, behavior)
	return Instance{
		Name:   "bespoke",
		Kind:   KindEndpoint,
		Source: SourceManifest,
		URL:    "https://airflow.acme.internal",
		Link:   execLink(helperArgv(extra...)),
	}
}

func TestExecMethodReadsTheTokenFromStdout(t *testing.T) {
	i := execInstance(t, "token")
	src, refresh, err := credentials(i, i.URL, Deps{LookupEnv: env(nil)})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if got := header(t, src); got != "Bearer s3cr3t-token" {
		t.Fatalf("header = %q, want the trimmed token", got)
	}
	if refresh == nil {
		t.Fatal("no refresh hook: a helper's token can expire mid-command like any other")
	}
}

// TestExecMethodPassesItsArgumentsThrough: the command is an argv array and
// runs directly, so its arguments arrive as written and no shell touches them.
func TestExecMethodPassesItsArgumentsThrough(t *testing.T) {
	i := execInstance(t, "args", "--profile", "prod one")
	src, _, err := credentials(i, i.URL, Deps{LookupEnv: env(nil)})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if got := header(t, src); got != "Bearer --profile|prod one" {
		t.Fatalf("header = %q, want the arguments unsplit", got)
	}
}

func TestExecMethodNamesEveryWayItCanFail(t *testing.T) {
	cases := []struct {
		behavior string
		want     []string
	}{
		{"fail", []string{`instance "bespoke"`, "exit status 3", "no SSO session"}},
		{"empty", []string{"printed no token"}},
		{"whitespace", []string{"printed no token"}},
		{"multiline", []string{"printed 2 lines", "a token is one"}},
	}
	for _, tc := range cases {
		i := execInstance(t, tc.behavior)
		src, _, err := credentials(i, i.URL, Deps{LookupEnv: env(nil)})
		if err != nil {
			t.Fatalf("%s: credentials: %v", tc.behavior, err)
		}
		_, _, err = src(context.Background())
		if err == nil {
			t.Fatalf("%s: the helper's answer was accepted", tc.behavior)
		}
		// Every message quotes the command, because running it is the reader's
		// next move.
		want := append([]string{"-test.run=^TestExecHelperProcess$"}, tc.want...)
		for _, phrase := range want {
			if !strings.Contains(err.Error(), phrase) {
				t.Errorf("%s: message does not name %q: %v", tc.behavior, phrase, err)
			}
		}
	}
}

func TestExecMethodBoundsAHelperThatHangs(t *testing.T) {
	t.Setenv(helperEnv, "hang")
	h := &execHelper{instance: "bespoke", argv: helperArgv(), timeout: 100 * time.Millisecond}
	_, _, err := h.credentials(context.Background())
	if err == nil || !strings.Contains(err.Error(), "did not finish within 100ms") {
		t.Fatalf("err = %v, want the timeout named", err)
	}
}

func TestExecMethodHoldsTheTokenUntilRefreshed(t *testing.T) {
	i := execInstance(t, "token")
	src, refresh, err := credentials(i, i.URL, Deps{LookupEnv: env(nil)})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if _, _, err := src(context.Background()); err != nil {
		t.Fatal(err)
	}
	// The helper is gone from the environment, so a second run of it would
	// print nothing and fail. It succeeding proves nothing ran.
	t.Setenv(helperEnv, "empty")
	if _, _, err := src(context.Background()); err != nil {
		t.Fatalf("the helper ran again for a token already in hand: %v", err)
	}
	if err := refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := src(context.Background()); err == nil {
		t.Fatal("a refresh did not re-run the helper")
	}
}

func TestExecMethodRefusesAnEmptyCommand(t *testing.T) {
	i := Instance{Name: "bespoke", Source: SourceManifest, URL: "https://af.corp", Link: execLink(nil)}
	if _, _, err := credentials(i, i.URL, Deps{}); err == nil {
		t.Fatal("an exec link with no command resolved")
	}
}
