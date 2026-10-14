package cmd

import (
	"bytes"
	stdcontext "context"
	"io"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/cliout/cliouttest"
	"github.com/astronomer/astro-cli/cmd/local"
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
	// `astro dev`'s stub, which the 1.x command guard runs under -o json
	// (removed_commands_test.go).
	reflect.TypeOf(local.DevRemoved{}): "cmd/local/testdata/schema/dev-removed.json",
}

// emitWatch is this package's configuration of the observer TestMain arms.
func emitWatch() cliouttest.Watch {
	return cliouttest.Watch{
		Cases:           publishedPayloads,
		PinnedElsewhere: pinnedElsewhere,
		Floor:           minWatchedPayloads,
		File:            "cmd/schema_test.go",
	}
}

// Every key a golden here publishes is snake_case: a capital in one is a Go
// field name that reached the wire because a tag was forgotten.
func TestPublishedKeysAreSnakeCase(t *testing.T) {
	assert.Empty(t, cliouttest.KeyProblems(t, schemaDir, len(publishedPayloads) > 0))
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
