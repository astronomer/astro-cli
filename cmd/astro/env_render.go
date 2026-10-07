package astro

// How the `astro env` writes look, in text and in json: a set publishes the
// object as it now is, a delete the object as it was, and a link change the
// object's links as the change left them, the report `link list` prints.
// The text of each is the line it always printed.

import (
	"bufio"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/env"
)

// envRenderer parses the -o every `astro env` command inherits from the
// group, text or json. The reads that offer more (dotenv) parse their own.
func envRenderer(out io.Writer) (cliout.Renderer, error) {
	f, err := cliout.ParseFormat(envOutput)
	if err != nil {
		return cliout.Renderer{}, err
	}
	return cliout.Renderer{Format: f, Out: out}, nil
}

// getFn matches the per-type GetVar / GetConn / GetAirflowVar /
// GetMetricsExport signature.
type getFn func(idOrKey string, scope env.Scope, includeSecrets bool, client astrov1.APIClient) (*astrov1.EnvironmentObject, error)

// createdAsHeld returns what a create left, as the platform holds it, for
// json to publish. The create endpoint answers with an id alone, so a Create*
// returns an object built from the inputs: it echoes a secret value back,
// and lacks what the platform fills in (set_fields, created_at). Under json
// the object is read back by that id instead, so a set publishes what a
// `get` would. Text prints only the key and the id, which the echo has, so
// it reads nothing more.
//
// The create has happened whether or not the read does, so a failed read
// does not fail the set: it says so on warn and returns the object the
// create built, with set_fields named from its inputs (env.WithSetFields),
// which every json path masks (setInfo).
func createdAsHeld(warn io.Writer, r cliout.Renderer, obj *astrov1.EnvironmentObject, scope env.Scope, get getFn) *astrov1.EnvironmentObject {
	if r.Format != cliout.FormatJSON || obj.Id == nil {
		return obj
	}
	held, err := get(*obj.Id, scope, false, astroV1Client)
	if err != nil {
		fmt.Fprintf(warn, "Created %s (id: %s), but reading it back failed (%s); showing it as created, secrets masked.\n", obj.ObjectKey, *obj.Id, err)
		return env.WithSetFields(obj)
	}
	return held
}

// setInfo is the object a set or a delete publishes, its secrets masked
// whatever the platform answered with (env.MaskSecrets): a secret's value is
// null, a password or token absent.
func setInfo(obj *astrov1.EnvironmentObject) env.ObjectInfo {
	return env.NewObjectInfo(env.MaskSecrets(obj), false)
}

// renderEnvSet renders the object a single set left: "Created KEY (id: ID)"
// or "Updated KEY" in text, the object in json.
func renderEnvSet(r cliout.Renderer, obj *astrov1.EnvironmentObject, created bool) error {
	info := setInfo(obj)
	return r.Emit(info, cliout.Text(func(b *bufio.Writer) { writeSetLine(b, &info, created) }))
}

// writeSetLine is the line a set prints for one object, with the id scripts
// read out of a create's. One definition because six paths print it, the
// four nouns and the two bulk imports.
func writeSetLine(w io.Writer, o *env.ObjectInfo, created bool) {
	if !created {
		fmt.Fprintf(w, "Updated %s\n", o.ObjectKey)
		return
	}
	id := ""
	if o.ID != nil {
		id = *o.ID
	}
	fmt.Fprintf(w, "Created %s (id: %s)\n", o.ObjectKey, id)
}

// renderEnvSetFromFile renders a `set --from-file`: a line per key it set,
// in key order, or that the file had none.
func renderEnvSetFromFile(r cliout.Renderer, res *env.SetFromFileResult, path string) error {
	return r.Emit(res, cliout.Text(func(b *bufio.Writer) {
		if len(res.Outcomes) == 0 {
			fmt.Fprintf(b, "no variables found in %s\n", displayPath(path))
			return
		}
		writeSetLines(b, res)
	}))
}

// writeSetLines prints a line for each key a --from-file set.
func writeSetLines(w io.Writer, res *env.SetFromFileResult) {
	for _, o := range res.Outcomes {
		if o.Object != nil {
			writeSetLine(w, o.Object, o.Kind == env.SetCreated)
		}
	}
}

// renderEnvDeleted renders a delete: "Deleted ID-OR-KEY" in text, and in json
// the object as it was before.
func renderEnvDeleted(r cliout.Renderer, obj *astrov1.EnvironmentObject, idOrKey string) error {
	return r.Emit(setInfo(obj), cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintf(b, "Deleted %s\n", idOrKey)
	}))
}

// linkChanged renders a link set or delete, line in text and in json report,
// the object's links as the change left them; then it says, on stderr, when
// deployments see the change.
func linkChanged(cmd *cobra.Command, r cliout.Renderer, report any, line string) error {
	if err := r.Emit(report, cliout.Text(func(b *bufio.Writer) { fmt.Fprintln(b, line) })); err != nil {
		return err
	}
	fmt.Fprintln(cmd.ErrOrStderr(), deploymentPickupNote)
	return nil
}
