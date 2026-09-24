package localdocker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// migrationFailure is a fake compose whose up fails and whose logs end the way
// `airflow db migrate` does on a database a newer Airflow upgraded — the tail
// of the traceback, verbatim from Airflow 3.1.8 starting on a 3.2.2 database.
func migrationFailure(logs ...string) *fakeCmd {
	cmd := &fakeCmd{output: noProjects}
	cmd.run = func(call string, s rt.Stdio) error {
		if strings.Contains(call, " up ") {
			return errors.New("exit status 1")
		}
		if strings.Contains(call, " logs ") && s.Out != nil {
			for _, l := range logs {
				_, _ = s.Out.Write([]byte("db-migration-1  | 2026-09-23T21:19:10.000000000Z " + l + "\n"))
			}
		}
		return nil
	}
	return cmd
}

var newerDatabaseTail = []string{
	`  File "/usr/local/lib/python3.12/site-packages/alembic/script/base.py", line 249, in _catch_revision_errors`,
	`    raise util.CommandError(resolution) from re`,
	`alembic.util.exc.CommandError: Can't locate revision identified by '1d6611b6ab7c'`,
}

// A database a newer Airflow upgraded is named as that, whether or not anything
// is streaming the logs: the CLI streams them, and a consumer passing no line
// callback would otherwise get the same "exit status 1" the CLI used to print.
func TestFailedStartNamesADatabaseANewerAirflowUpgraded(t *testing.T) {
	for _, tc := range []struct {
		name string
		cb   rt.Callbacks
	}{
		{"streaming", rt.Callbacks{OnLine: func(rt.LogLine) {}}},
		{"not streaming", rt.Callbacks{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := testEngine(t, migrationFailure(newerDatabaseTail...))

			_, err := e.Start(context.Background(), testPlan(t), tc.cb)
			require.Error(t, err)
			assert.ErrorIs(t, err, airflowrt.ErrDatabaseNewerThanAirflow)
			assert.Contains(t, err.Error(), "1d6611b6ab7c", "the error should name the revision Airflow could not find")
			assert.NotContains(t, err.Error(), "starting project containers", "the precise cause replaces the generic one")
		})
	}
}

// Any other migration failure keeps the generic error. Calling every traceback
// a newer database would send someone to wipe their data over a typo in a
// plugin.
func TestOtherMigrationFailuresAreNotCalledANewerDatabase(t *testing.T) {
	e := testEngine(t, migrationFailure(
		`sqlalchemy.exc.OperationalError: could not connect to server: Connection refused`,
	))

	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{OnLine: func(rt.LogLine) {}})
	require.Error(t, err)
	assert.NotErrorIs(t, err, airflowrt.ErrDatabaseNewerThanAirflow)
	assert.Contains(t, err.Error(), "starting project containers")
}
