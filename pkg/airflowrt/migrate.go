package airflowrt

import (
	"errors"
	"regexp"
)

// ErrDatabaseNewerThanAirflow reports an Airflow that refused its metadata
// database because a newer Airflow had already upgraded it.
//
// Airflow migrates forward only. A project that ran on a newer Airflow and then
// pins an older one keeps the upgraded database, and the older Airflow's
// migration step fails on a revision it has never heard of. What reached a
// person was an alembic traceback and "exit status 1", which says nothing about
// the pin that caused it or the ways out.
//
// A sentinel, for the reason ErrHealthTimeout gives: this module is also what
// Astro Desktop builds on, so it reports what happened and leaves naming the
// way to start over to whoever owns one. The CLI names `astro local reset`.
var ErrDatabaseNewerThanAirflow = errors.New("the Airflow metadata database was upgraded by a newer Airflow than this project runs")

// unknownRevisionLine is alembic's refusal of a revision missing from the
// migrations it knows, which is what `airflow db migrate` reports for a
// database a newer Airflow has upgraded.
var unknownRevisionLine = regexp.MustCompile(`Can't locate revision identified by '([0-9A-Za-z_]+)'`)

// UnknownMigrationRevision reports the revision a line of Airflow's migration
// output says it cannot find, and whether the line says so at all.
func UnknownMigrationRevision(line string) (string, bool) {
	m := unknownRevisionLine.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	return m[1], true
}
