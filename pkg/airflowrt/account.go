package airflowrt

// The Airflow 2 admin account every local engine provisions.
//
// Airflow 2 has no all-admins mode: Flask-AppBuilder needs a real user row
// before anything can log in or mint a token. Every caller that authenticates
// against a local Airflow 2 has to send what created it, because a mismatch is a
// 401 on every request rather than a build error.
//
// Three writers create this account:
//
//   - v2 docker mode, from the database service's `airflow users create`
//   - v1 docker mode, from the compose template's `airflow users create`
//     (airflow/include/airflow2/composeyml.go.tmpl)
//   - standalone on macOS, from AF2DarwinShim's embedded Python (see
//     standalone_scripts/af2_darwin_shim.py)
//
// Standalone anywhere else is NOT a writer: `airflow standalone` generates its
// own password into standalone_admin_password.txt. A reader may fall back to
// this pair when that file is missing or blank (pkg/instances), which is a
// fallback and not a guarantee — code that sends this pair unconditionally to a
// non-macOS standalone Airflow 2 will 401. An earlier version of this comment
// called that third case a writer, which is exactly the mistake that produces
// such a caller
//
// It lives in this package because this is the only leaf that reaches all three.
// The pair started out in pkg/localrt/rt, which the docker engine and the CLI can
// both import — but not the shim, since pkg/localrt imports pkg/airflowrt and the
// reverse would be a cycle. Placing it where the shim cannot see it would have
// left the one writer with no compiler path to the constant and no test pinning
// it against the others, which is the failure the consolidation existed to
// prevent.
//
// Not a secret: fixed, published, local-only development credentials. Airflow's
// own `users create` takes them on a command line.
//
// Known gap, not covered by any test here: otto's v1 detection path
// (internal/otto/config.go) hands out this pair for any Airflow it finds through
// the v1 proxy routes, including a non-macOS standalone Airflow 2 whose password
// was generated. The v2 path refuses that case explicitly; the v1 path has no
// Airflow-major or mode information to refuse it with.
const (
	Airflow2AdminUser     = "admin"
	Airflow2AdminPassword = "admin"
)
