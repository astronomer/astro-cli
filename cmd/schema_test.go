package cmd

import (
	"bytes"
	stdcontext "context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/cliout/cliouttest"
	"github.com/astronomer/astro-cli/context"
)

// The shapes the commands this package builds itself publish under `--output
// json`, each pinned by a golden in testdata/schema, the same way cmd/local
// and cmd/astro pin their own (see cliouttest). These are the commands that
// sit outside both platform trees: `astro version`, `context`, `config`,
// `telemetry` and `auth token`. `make update-schemas` rewrites the goldens;
// read the diff before committing it, since a changed golden is a changed
// contract.
//
// The goldens pin the shape once. A command's own tests then decode what it
// printed and assert what it means, rather than restating these bytes.

// schemaDir holds this package's goldens.
var schemaDir = filepath.Join("testdata", "schema")

// publishedPayloads is every shape a command built here writes under
// `--output json`, named for the surface rather than the Go type.
var publishedPayloads = []cliouttest.Case{
	// astro version.
	{Name: "version", Value: versionOutput{}},
	// astro context list, and the one context `context switch` made current,
	// as the list shows it. delete: what it deleted.
	{Name: "context-list", Value: context.InfoList{}},
	{Name: "context", Value: context.Info{}},
	{Name: "context-removal", Value: context.Removal{}},
	// astro config get and set: the one setting; list: every setting.
	{Name: "config-setting", Value: configSetting{}},
	{Name: "config-list", Value: configSettings{}},
	// astro telemetry, enable and disable: the state each leaves.
	{Name: "telemetry", Value: telemetryState{}},
	// astro auth token. expires_at is absent when the login does not say.
	{Name: "auth-token", Value: authToken{}},
}

func TestPublishedJSONPayloadsKeepTheirShape(t *testing.T) {
	for _, c := range publishedPayloads {
		t.Run(c.Name, func(t *testing.T) { cliouttest.Check(t, schemaDir, c) })
	}
}

func TestEveryGoldenHasACase(t *testing.T) {
	assert.Empty(t, cliouttest.Orphans(t, schemaDir, publishedPayloads),
		"these goldens have no case in publishedPayloads, so nothing regenerates\n"+
			"them and nothing would notice the shape they pin going stale. If the\n"+
			"payload is gone, delete the file; if a case was renamed, delete the\n"+
			"file the old name left behind.")
}

// What the commands actually emit, watched at the door, as cmd/astro and
// cmd/local watch theirs: TestMain arms cliout.EmitObserver for the package
// run, and fails it when a named type reached Emit that no golden pins, or an
// anonymous struct reached it at all. Blind to what the suite does not run.

// minWatchedPayloads is a floor under the tally, not a target: an empty tally
// reads exactly like a clean one, so without it the observer coming unwired
// would be silent. Nine shapes reach Emit in this package's tests today: the
// version, the context list, the one context and a context's removal, a
// config setting and the config list, the telemetry state, the auth token,
// and the error object.
// Raise it as conversions land; lower it only saying why.
const minWatchedPayloads = 9

// pinnedElsewhere names the shapes that reach Emit here and are pinned by
// another tree's goldens, keyed by type, with where.
var pinnedElsewhere = map[reflect.Type]string{
	// The failure object cliout.Execute publishes for every command, whichever
	// tree it is in.
	reflect.TypeOf(cliout.ErrorObject{}): "cmd/local/testdata/schema/error.json",
}

var emitted = struct {
	sync.Mutex
	named     map[reflect.Type]bool
	anonymous map[string]bool
}{
	named:     map[reflect.Type]bool{},
	anonymous: map[string]bool{},
}

func recordEmitted(v any) {
	if v == nil {
		return
	}
	t := cliouttest.PayloadType(reflect.TypeOf(v))
	if t == nil {
		return
	}
	emitted.Lock()
	defer emitted.Unlock()
	if t.Name() == "" {
		emitted.anonymous[t.String()] = true
		return
	}
	emitted.named[t] = true
}

// watchEmit arms the observer and returns what reports on it after the run.
// This package's TestMain honors -run, and a run it narrowed has a partial
// tally by definition, so the floor applies only to a whole run; whatever did
// run and emitted something unpinned is still reported.
func watchEmit() (problems func() []string) {
	cliout.EmitObserver = recordEmitted
	return func() []string {
		emitted.Lock()
		defer emitted.Unlock()
		floor := minWatchedPayloads
		if f := flag.Lookup("test.run"); f != nil && f.Value.String() != "" {
			floor = 0
		}
		return emitProblems(emitted.named, emitted.anonymous, floor)
	}
}

// emitProblems is the report over a tally handed in, so it can be tested
// directly.
func emitProblems(named map[reflect.Type]bool, anonymous map[string]bool, floor int) []string {
	pinned := map[reflect.Type]bool{}
	for _, c := range publishedPayloads {
		if t := cliouttest.PayloadType(reflect.TypeOf(c.Value)); t != nil {
			pinned[t] = true
		}
	}

	var out []string
	if len(named) < floor {
		out = append(out, fmt.Sprintf(
			"only %d payload shapes were seen reaching cliout.Renderer.Emit, below the floor of %d.\n"+
				"    Either the observer is no longer wired up, in which case the check\n"+
				"    below is passing on an empty tally, or command tests stopped running.\n"+
				"    If the drop is real and intended, lower minWatchedPayloads and say why.",
			len(named), floor))
	}
	for t := range named {
		if !pinned[t] && pinnedElsewhere[t] == "" {
			out = append(out, fmt.Sprintf(
				"a command passed %s through cliout.Renderer.Emit and no golden pins it.\n"+
					"    Add it to publishedPayloads in cmd/schema_test.go and run\n"+
					"    `make update-schemas`; a shape that reaches stdout is a contract.", t))
		}
	}
	for name := range anonymous {
		out = append(out, fmt.Sprintf(
			"a command passed the anonymous struct %s through cliout.Renderer.Emit.\n"+
				"    An anonymous shape cannot be pinned. Give it a name and add it to\n"+
				"    publishedPayloads.", name))
	}
	sort.Strings(out)
	return out
}

func TestEmitProblemsReportsWhatIsUnpinned(t *testing.T) {
	type notPinned struct {
		A string `json:"a"`
	}
	got := emitProblems(
		map[reflect.Type]bool{reflect.TypeOf(notPinned{}): true},
		map[string]bool{"struct { A int }": true},
		99,
	)
	assert.Len(t, got, 3, "the floor, the named type and the anonymous one")

	pinned := map[reflect.Type]bool{reflect.TypeOf(authToken{}): true}
	assert.Empty(t, emitProblems(pinned, nil, 1), "a pinned type, above the floor")
}

// textTo is a text renderer onto w, for a test calling a command's function
// directly.
func textTo(w io.Writer) cliout.Renderer {
	return cliout.Renderer{Format: cliout.FormatText, Out: w}
}

// runCommands runs args the way main runs the CLI (cliout.Execute), against a
// root holding only the commands build returns, each built on the stdout the
// run reports on. It returns what reached stdout and stderr.
func runCommands(build func(out io.Writer) []*cobra.Command, args ...string) (stdout, stderr string, err error) {
	var out, errOut bytes.Buffer
	root := &cobra.Command{Use: "astro"}
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.AddCommand(build(&out)...)
	err = cliout.Execute(stdcontext.Background(), root, args, &out, problemKinds)
	return out.String(), errOut.String(), err
}
