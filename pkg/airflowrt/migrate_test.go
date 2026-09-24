package airflowrt

import "testing"

func TestUnknownMigrationRevision(t *testing.T) {
	cases := []struct {
		name string
		line string
		rev  string
		ok   bool
	}{
		{
			// Verbatim from Airflow 3.1.8 starting on a database Airflow 3.2.2
			// had upgraded.
			name: "alembic refusing a newer database",
			line: "alembic.util.exc.CommandError: Can't locate revision identified by '1d6611b6ab7c'",
			rev:  "1d6611b6ab7c",
			ok:   true,
		},
		{
			name: "the rest of the traceback",
			line: `  File "/usr/local/lib/python3.12/site-packages/alembic/script/base.py", line 249, in _catch_revision_errors`,
		},
		{
			name: "an ordinary migration line",
			line: "Running upgrade 29ce7909c52b -> 1d6611b6ab7c, add bundle_name to callback table",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rev, ok := UnknownMigrationRevision(tc.line)
			if rev != tc.rev || ok != tc.ok {
				t.Errorf("UnknownMigrationRevision() = (%q, %v), want (%q, %v)", rev, ok, tc.rev, tc.ok)
			}
		})
	}
}
