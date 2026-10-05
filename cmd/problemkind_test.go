package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	pkgerrors "github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/telemetry"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

// apiFailure is the error an Astro API call returns for status, built by the
// function every call goes through.
func apiFailure(status int) error {
	return httputil.NormalizeAPIError(&http.Response{StatusCode: status}, []byte(`{"message":"refused"}`))
}

// cloudSamples is one real error per cloud kind; every test below works from
// it, and a kind with no sample fails.
var cloudSamples = map[cliout.ProblemKind][]error{
	KindUnauthenticated: {
		apiFailure(http.StatusUnauthorized),
		// The v1 tree's "no context" — what a never-logged-in machine gets —
		// arrives wrapped by github.com/pkg/errors, as cmd/astro wraps it.
		pkgerrors.Wrap(config.ErrGetHomeString, "failed to get current Workspace"),
		astrosession.ErrLoggedOut,
	},
	KindForbidden:      {apiFailure(http.StatusForbidden)},
	KindNotFound:       {apiFailure(http.StatusNotFound)},
	KindConflict:       {apiFailure(http.StatusConflict)},
	KindAPIUnavailable: {apiFailure(http.StatusInternalServerError), apiFailure(http.StatusServiceUnavailable)},
}

func TestEveryCloudKindResolvesThroughAWrap(t *testing.T) {
	for _, rule := range cloudKinds {
		samples := cloudSamples[rule.Kind]
		if len(samples) == 0 {
			t.Errorf("no sample error for %q; add one so the kind is exercised", rule.Kind)
		}
		for _, sample := range samples {
			wrapped := fmt.Errorf("listing deployments: %w", sample)
			if got := problemKinds.Of(wrapped); got != rule.Kind {
				t.Errorf("problemKinds.Of(%q) = %q, want %q", wrapped, got, rule.Kind)
			}
		}
	}
}

// A status the CLI has no name for publishes no kind, rather than a guess.
func TestAnUnnamedStatusHasNoKind(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusTooManyRequests} {
		if got := problemKinds.Of(apiFailure(status)); got != "" {
			t.Errorf("status %d resolves to %q, want no kind", status, got)
		}
	}
}

func TestCloudKindOrderIsPinned(t *testing.T) {
	want := []cliout.ProblemKind{KindUnauthenticated, KindForbidden, KindNotFound, KindConflict, KindAPIUnavailable}
	if len(cloudKinds) != len(want) {
		t.Fatalf("%d kinds in the table, %d pinned here", len(cloudKinds), len(want))
	}
	for i, r := range cloudKinds {
		if r.Kind != want[i] {
			t.Errorf("row %d is %q, pinned as %q", i, r.Kind, want[i])
		}
	}
}

// No two tables name the same kind: a name means one thing across the CLI.
func TestNoKindIsNamedTwice(t *testing.T) {
	seen := map[cliout.ProblemKind]bool{cliout.KindUsage: true}
	for _, r := range problemKinds {
		if seen[r.Kind] {
			t.Errorf("kind %q is named twice", r.Kind)
		}
		seen[r.Kind] = true
		k := string(r.Kind)
		if k != strings.ToLower(k) || strings.ContainsAny(k, " -.") {
			t.Errorf("kind %q should be lower snake_case", k)
		}
	}
}

// executeRoot runs args against an assembled root the way main does.
func executeRoot(root *cobra.Command, args ...string) (stdout, stderr string, err error) {
	var out, errOut bytes.Buffer
	root.SetOut(&errOut)
	root.SetErr(&errOut)
	err = cliout.Execute(context.Background(), root, args, &out, problemKinds)
	return out.String(), errOut.String(), err
}

// The contract is applied at the root, so it reaches every command in both
// trees, not only the v2 one.
func TestACloudCommandFailsAsOneJSONObject(t *testing.T) {
	for platform, root := range rootsUnderTest(t) {
		t.Run(platform, func(t *testing.T) {
			// A cloud leaf that has --output json, failing the way an API call
			// does. The pre-run is skipped because it reads the machine's login.
			list, _, err := root.Find([]string{"deployment", "list"})
			if err != nil {
				t.Fatal(err)
			}
			if !reachesJSONOutput(list) {
				// APC's has none yet; give it one, since the point is the root.
				var output string
				cliout.AddOutputFlag(list, &output)
			}
			list.Annotations = map[string]string{telemetry.SkipPreRunAnnotation: "true"}
			list.RunE = func(*cobra.Command, []string) error {
				return fmt.Errorf("listing: %w", apiFailure(http.StatusUnauthorized))
			}

			stdout, stderr, err := executeRoot(root, "deployment", "list", "-o", "json")
			if err == nil {
				t.Fatal("the command should fail")
			}
			var obj cliout.ErrorObject
			if jerr := json.Unmarshal([]byte(stdout), &obj); jerr != nil || strings.Count(stdout, "\n") != 1 {
				t.Fatalf("stdout is not one error object: %q (%v)", stdout, jerr)
			}
			if obj.Kind != KindUnauthenticated || obj.Code != cliout.ExitFailure {
				t.Errorf("got %+v, want kind %q and code %d", obj, KindUnauthenticated, cliout.ExitFailure)
			}
			if stderr != "" {
				t.Errorf("json mode wrote to stderr: %q", stderr)
			}
		})
	}
}

// A usage error exits 2 anywhere in the tree, and says so in json mode.
func TestUsageErrorsAcrossTheTree(t *testing.T) {
	cases := [][]string{
		{"deployment", "list", "--bogus"},
		{"local", "status", "--bogus"},
		{"bogus"},
	}
	for platform, root := range rootsUnderTest(t) {
		for _, args := range cases {
			t.Run(platform+" "+strings.Join(args, " "), func(t *testing.T) {
				_, stderr, err := executeRoot(root, args...)
				if code := cliout.ExitCode(context.Background(), err); code != cliout.ExitUsage {
					t.Errorf("exit %d, want %d (err %v)", code, cliout.ExitUsage, err)
				}
				if !strings.Contains(stderr, "Error: ") {
					t.Errorf("text mode should print the error: %q", stderr)
				}
			})
		}
	}
}

func TestAJSONUsageErrorAtTheRoot(t *testing.T) {
	for platform, root := range rootsUnderTest(t) {
		t.Run(platform, func(t *testing.T) {
			stdout, _, err := executeRoot(root, "local", "status", "--bogus", "-o", "json")
			if cliout.ExitCode(context.Background(), err) != cliout.ExitUsage {
				t.Fatalf("want a usage error, got %v", err)
			}
			var obj cliout.ErrorObject
			if jerr := json.Unmarshal([]byte(stdout), &obj); jerr != nil {
				t.Fatalf("stdout is not an error object: %q (%v)", stdout, jerr)
			}
			if obj.Kind != cliout.KindUsage || obj.Code != cliout.ExitUsage {
				t.Errorf("got %+v", obj)
			}
		})
	}
}
