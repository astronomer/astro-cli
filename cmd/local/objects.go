package local

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/airflowenv"
)

// envSourcedNote says why a connection or Variable can work in a task and still
// be missing here. Airflow's API reads only its database, and `astro local env`
// and the platform both hand values to Airflow as environment variables, so a
// value someone just set is exactly the one these commands cannot see.
func (q *query) envSourcedNote(kind localenv.Kind) string {
	what, prefix := "Connections", airflowenv.ConnPrefix
	if kind == localenv.KindVar {
		what, prefix = "Airflow variables", airflowenv.VarPrefix
	}
	note := fmt.Sprintf("%s set as environment variables (%s*) are not in Airflow's database, so they do not "+
		"appear here.", what, prefix)
	list := q.t.envList(localenv.Noun(kind), q.opened)
	if list == "" {
		return note
	}
	return note + " To see the ones this CLI manages, run `" + list + "`"
}

// noteEnvSourced closes a text listing with envSourcedNote on stderr.
func (q *query) noteEnvSourced(r cliout.Renderer, kind localenv.Kind) {
	if r.Format == cliout.FormatText {
		fmt.Fprintln(q.d.Stderr, "note: "+q.envSourcedNote(kind))
	}
}

// notFoundEnvSourced adds envSourcedNote to a not-found from a get, keeping the
// error it wraps.
func (q *query) notFoundEnvSourced(err error, kind localenv.Kind) error {
	if !errors.Is(err, airflowapi.ErrNotFound) {
		return err
	}
	return fmt.Errorf("%w\n%s", err, q.envSourcedNote(kind))
}

// newConnectionsCmd builds the `connections` family over whichever Airflow the
// target names. It and the two
// families below are the Airflow objects this surface reads but never writes:
// setting them belongs to `astro local env` on the machine, and to the platform
// commands on a deployment.
func newConnectionsCmd(d Deps, t target) *cobra.Command {
	return newQueryCmd(d, t, &cobra.Command{
		Use:   "connections",
		Short: "List and read the connections on an Airflow",
		Long: "Read the connections on " + t.which() + ": which systems it can reach, " +
			"and how.\n\nNo password is ever shown. The client this runs on does not decode the field at all, " +
			"so a password cannot reach a table, a log, or a json stream by accident — not with --output json, " +
			"not on `get`. Read one with `" + rawAPIForm(t) + "` if you genuinely need it.",
	},
		newConnectionsListCmd,
		newConnectionsGetCmd,
	)
}

// connectionListRow is a connection in a listing: where it points, and nothing
// that could be a credential. Extra is deliberately absent — it is a free-form
// blob that routinely holds tokens, keys, and passwords under names Airflow
// does not mask, so listing every connection on an instance must not hand them
// out. The family's no-secrets promise has to hold for --output json, not just
// for the table.
type connectionListRow struct {
	ConnectionID string `json:"connection_id"`
	ConnType     string `json:"conn_type,omitempty"`
	Host         string `json:"host,omitempty"`
	Port         int    `json:"port,omitempty"`
	Schema       string `json:"schema,omitempty"`
	Login        string `json:"login,omitempty"`
	Description  string `json:"description,omitempty"`
}

// connectionRow is one connection read on purpose, Extra included. Asking for a
// single connection by name is a deliberate act, the way `astro af variables get`
// is; the password is still absent, because the client never decodes it.
type connectionRow struct {
	connectionListRow
	Extra string `json:"extra,omitempty"`
}

func newConnectionListRow(c airflowapi.Connection) connectionListRow {
	return connectionListRow{
		ConnectionID: c.ConnectionID,
		ConnType:     c.ConnType,
		Host:         c.Host,
		Port:         c.Port,
		Schema:       c.Schema,
		Login:        c.Login,
		Description:  c.Description,
	}
}

func newConnectionsListCmd(q *query) *cobra.Command {
	var list listFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the connections on this Airflow",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return q.runConnectionsList(cmd.Context(), list.options())
		},
	}
	addListFlags(cmd, &list, "")
	return cmd
}

func (q *query) runConnectionsList(ctx context.Context, opts airflowapi.ListOptions) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	err = emitList(q, r, opts, func(page airflowapi.ListOptions) ([]airflowapi.Connection, int, error) {
		list, err := client.ListConnections(ctx, page)
		return list.Connections, list.TotalEntries, err
	}, newConnectionListRow, renderConnectionTable)
	if err != nil {
		return err
	}
	q.noteEnvSourced(r, localenv.KindConn)
	return nil
}

func renderConnectionTable(w io.Writer, rows []connectionListRow) error {
	return renderTable(w, rows, "No connections on this Airflow.",
		[]string{"CONN_ID", "TYPE", "HOST", "PORT", "SCHEMA", "LOGIN"},
		func(row connectionListRow) []string {
			return []string{row.ConnectionID, row.ConnType, row.Host, omitZero(row.Port), row.Schema, row.Login}
		})
}

func newConnectionsGetCmd(q *query) *cobra.Command {
	return &cobra.Command{
		Use:   "get <CONN_ID>",
		Short: "Show one connection, without its password",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return q.runConnectionsGet(cmd.Context(), args[0])
		},
	}
}

func (q *query) runConnectionsGet(ctx context.Context, id string) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	conn, err := client.GetConnection(ctx, id)
	if err != nil {
		return q.notFoundEnvSourced(err, localenv.KindConn)
	}
	row := connectionRow{connectionListRow: newConnectionListRow(conn), Extra: conn.Extra}
	return emitDetail(r, row, func(row connectionRow) []field {
		return []field{
			{"connection id", row.ConnectionID},
			{"type", row.ConnType},
			{"host", row.Host},
			{"port", omitZero(row.Port)},
			{"schema", row.Schema},
			{"login", row.Login},
			{"description", row.Description},
			{"extra", row.Extra},
		}
	})
}

// newVariablesCmd builds the `variables` family over whichever Airflow the
// target names.
func newVariablesCmd(d Deps, t target) *cobra.Command {
	return newQueryCmd(d, t, &cobra.Command{
		Use:   "variables",
		Short: "List and read the Airflow Variables on an Airflow",
		Long: "Read the Airflow Variables on " + t.which() + ".\n\n" +
			"`list` shows keys and descriptions but no values: a Variable holds whatever someone put in it, " +
			"and printing every value to answer \"what variables are there\" is how a secret ends up in a " +
			"terminal scrollback. Read one deliberately with `variables get <KEY>`.",
	},
		newVariablesListCmd,
		newVariablesGetCmd,
	)
}

// variableListRow is a Variable in a listing: no value, deliberately.
type variableListRow struct {
	Key         string `json:"key"`
	Description string `json:"description,omitempty"`
	IsEncrypted bool   `json:"is_encrypted"`
}

// variableRow is one Variable read on purpose, value and all.
type variableRow struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	IsEncrypted bool   `json:"is_encrypted"`
}

func newVariablesListCmd(q *query) *cobra.Command {
	var list listFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the Variable keys on this Airflow, without their values",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return q.runVariablesList(cmd.Context(), list.options())
		},
	}
	addListFlags(cmd, &list, "")
	return cmd
}

func (q *query) runVariablesList(ctx context.Context, opts airflowapi.ListOptions) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	err = emitList(q, r, opts, func(page airflowapi.ListOptions) ([]airflowapi.Variable, int, error) {
		list, err := client.ListVariables(ctx, page)
		return list.Variables, list.TotalEntries, err
	}, func(v airflowapi.Variable) variableListRow {
		return variableListRow{Key: v.Key, Description: v.Description, IsEncrypted: v.IsEncrypted}
	}, renderVariableTable)
	if err != nil {
		return err
	}
	q.noteEnvSourced(r, localenv.KindVar)
	return nil
}

func renderVariableTable(w io.Writer, rows []variableListRow) error {
	return renderTable(w, rows, "No Variables on this Airflow.",
		[]string{"KEY", "DESCRIPTION"},
		func(row variableListRow) []string { return []string{row.Key, row.Description} })
}

func newVariablesGetCmd(q *query) *cobra.Command {
	return &cobra.Command{
		Use:   "get <KEY>",
		Short: "Show one Variable and its value",
		Long: "Print one Variable's value. Airflow masks the values of Variables whose keys look sensitive, and " +
			"what comes back is whatever it sent.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return q.runVariablesGet(cmd.Context(), args[0])
		},
	}
}

func (q *query) runVariablesGet(ctx context.Context, key string) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	variable, err := client.GetVariable(ctx, key)
	if err != nil {
		return q.notFoundEnvSourced(err, localenv.KindVar)
	}
	row := variableRow{
		Key:         variable.Key,
		Value:       variable.Value,
		Description: variable.Description,
		IsEncrypted: variable.IsEncrypted,
	}
	return emitDetail(r, row, func(row variableRow) []field {
		return []field{
			{"key", row.Key},
			{"value", row.Value},
			{"description", row.Description},
			{"encrypted", onlyIf(row.IsEncrypted, "yes")},
		}
	})
}

// newPoolsCmd builds the `pools` family over whichever Airflow the target
// names.
func newPoolsCmd(d Deps, t target) *cobra.Command {
	return newQueryCmd(d, t, &cobra.Command{
		Use:   "pools",
		Short: "List and read the concurrency pools on an Airflow",
		Long: "Read the pools on " + t.which() + ": how many slots each has, and how " +
			"many of them tasks are sitting in right now. A pool with no open slots is why a task is queued.",
	},
		newPoolsListCmd,
		newPoolsGetCmd,
	)
}

// poolRow is a concurrency pool as this surface reports it.
type poolRow struct {
	Name            string `json:"name"`
	Slots           int    `json:"slots"`
	OccupiedSlots   int    `json:"occupied_slots"`
	RunningSlots    int    `json:"running_slots"`
	QueuedSlots     int    `json:"queued_slots"`
	ScheduledSlots  int    `json:"scheduled_slots"`
	DeferredSlots   int    `json:"deferred_slots"`
	OpenSlots       int    `json:"open_slots"`
	Description     string `json:"description,omitempty"`
	IncludeDeferred bool   `json:"include_deferred"`
}

func newPoolRow(p airflowapi.Pool) poolRow {
	return poolRow{
		Name:            p.Name,
		Slots:           p.Slots,
		OccupiedSlots:   p.OccupiedSlots,
		RunningSlots:    p.RunningSlots,
		QueuedSlots:     p.QueuedSlots,
		ScheduledSlots:  p.ScheduledSlots,
		DeferredSlots:   p.DeferredSlots,
		OpenSlots:       p.OpenSlots,
		Description:     p.Description,
		IncludeDeferred: p.IncludeDeferred,
	}
}

func newPoolsListCmd(q *query) *cobra.Command {
	var list listFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the pools on this Airflow",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return q.runPoolsList(cmd.Context(), list.options())
		},
	}
	addListFlags(cmd, &list, "")
	return cmd
}

func (q *query) runPoolsList(ctx context.Context, opts airflowapi.ListOptions) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	return emitList(q, r, opts, func(page airflowapi.ListOptions) ([]airflowapi.Pool, int, error) {
		list, err := client.ListPools(ctx, page)
		return list.Pools, list.TotalEntries, err
	}, newPoolRow, renderPoolTable)
}

func renderPoolTable(w io.Writer, rows []poolRow) error {
	return renderTable(w, rows, "No pools on this Airflow.",
		[]string{"NAME", "SLOTS", "RUNNING", "QUEUED", "SCHEDULED", "OPEN"},
		func(row poolRow) []string {
			return []string{
				row.Name,
				count(row.Slots),
				count(row.RunningSlots),
				count(row.QueuedSlots),
				count(row.ScheduledSlots),
				count(row.OpenSlots),
			}
		})
}

func newPoolsGetCmd(q *query) *cobra.Command {
	return &cobra.Command{
		Use:   "get <NAME>",
		Short: "Show one pool's slots and how they are used",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return q.runPoolsGet(cmd.Context(), args[0])
		},
	}
}

func (q *query) runPoolsGet(ctx context.Context, name string) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	pool, err := client.GetPool(ctx, name)
	if err != nil {
		return err
	}
	return emitDetail(r, newPoolRow(pool), func(row poolRow) []field {
		return []field{
			{"name", row.Name},
			{"description", row.Description},
			{"slots", count(row.Slots)},
			{"occupied", count(row.OccupiedSlots)},
			{"running", count(row.RunningSlots)},
			{"queued", count(row.QueuedSlots)},
			{"scheduled", count(row.ScheduledSlots)},
			{"deferred", count(row.DeferredSlots)},
			{"open", count(row.OpenSlots)},
			{"counts deferred", onlyIf(row.IncludeDeferred, "yes")},
		}
	})
}
