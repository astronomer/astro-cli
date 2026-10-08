package rt

import "time"

// ExternalRuntime describes a local Airflow that something other than this
// module supervises: the consumer started the process, owns its lifecycle, and
// is reporting it so the rest of the toolchain can see it.
//
// It exists because the runtime record is the interop contract — "tools
// coordinate through disk, not through each other" (docs/architecture.md) —
// and until now only an engine inside pkg/localrt could write one. Astro Desktop
// supervises standalone Airflow itself, with its own process supervisor, restart
// policy and idle-cool timer, so it had no way to publish a record at all. Two
// consequences, both of which this type is here to end: `astro local list` and
// `astro local status` could not see a desktop-started Airflow, and
// `astro local start` would stand up a SECOND one against the same project
// directory, because the live-start guard had no record to read.
//
// This is the same shape of gap that moving Runtime out of cmd/local closed
// one level up. The dispatch, the
// start lock and the refuse-a-live-start rules were CLI-private, so a second
// consumer had no way to reach them without reimplementing them differently;
// see Runtime's own doc comment. The record writer was the next layer of that.
//
// Deliberately NOT the on-disk record type. That type carries fields only an
// engine can fill (the compose project name), fields it derives rather than
// accepts (the start timestamp), and it is free to grow — publishing it as API
// would make every future record field a breaking change for the consumer. The
// narrow input type is the trade Plan already makes for Start.
//
// # What a claim does not buy
//
// Publishing a record makes a runtime VISIBLE. It does not make every
// record-driven command correct for it, because those commands were written for
// runtimes this module started:
//
//   - `astro local logs` reads the log file the engine writes under the
//     project's state dir. A consumer with its own log pipeline writes no such
//     file, so the command reports having no logs for an Airflow that is
//     demonstrably running.
//   - `astro local run` and `astro local shell` build their environment from
//     the project's own .venv and this module's own conventions, so they may
//     address a different metadata database than the claimed Airflow.
//   - Routes and the proxy daemon are the consumer's to manage. Claim records a
//     Hostname but registers nothing in routes.json, and Release removes the
//     record but neither the route nor the daemon.
//   - `astro local stop` acts on a claimed runtime as if it owned it: it signals
//     the recorded group, removes the record, and drops the route. That is
//     deliberate interop — either tool should be able to stop what the other
//     started — but a consumer with a restart policy will see its child die and
//     bring Airflow back up, now with no record and no lock, which is precisely
//     the invisible-Airflow state this type exists to end. There is no signal to
//     subscribe to and no field marking a runtime externally supervised, so a
//     consumer that restarts automatically has to reconcile against
//     RecordedStatus rather than trusting its own child's death.
//
// Fixing those properly means marking the record as externally supervised and
// teaching each command to defer, which changes this module's own commands and
// is deliberately not bundled here. Until then a consumer should expect to own
// logs, shells and routes for anything it claims, and to re-check the record
// after any unexpected exit.
type ExternalRuntime struct {
	// ProjectPath is the project root. Made absolute on the way in, matching
	// what the engines store, so a claimed record and an engine-written one are
	// the same spelling of the same directory.
	ProjectPath string

	// Mode is the runtime being reported. Only ModeStandalone is accepted:
	// docker mode runs through this module's own engine, which writes its own
	// record, so an external docker claim would either duplicate that or
	// describe containers nothing here can stop. Empty means ModeStandalone.
	Mode Mode

	// PID is the supervised Airflow process.
	PID int

	// Pgid is the process group of the supervised Airflow, and it must be a
	// group the CONSUMER STARTED. That restriction is the whole subtlety of
	// this type, so it is worth being exact about.
	//
	// The record's group is not only a liveness token, it is a kill target: a
	// stop resolves Record.GroupID and sends SIGTERM, then SIGKILL, to that
	// group. For an engine-written record the two coincide, because the engine
	// started the group it recorded. A consumer that records a group it did not
	// start — its own process group, say, to hold a claim open before Airflow
	// exists — makes `astro local stop` fire SIGKILL into the consumer itself
	// and everything sharing its group.
	//
	// So there is no supported way to claim a project before there is an Airflow
	// process group to name. Reserve exists for that window instead: it holds
	// the project's start lock, which keeps a competing start out without
	// putting a group nothing here may signal onto disk.
	//
	// Zero means "PID leads its own group", which is what a consumer that starts
	// Airflow with Setpgid gets. Claim resolves and writes the value out rather
	// than leaving the zero on disk, and refuses a claim whose group is not
	// actually alive — because a record naming a dead or unsignalable group
	// reads as stopped however healthy Airflow is, and then the project vanishes
	// from `astro local list` and a start stops refusing, which is the failure
	// this type exists to prevent, restored quietly.
	Pgid int

	// Port is where Airflow listens. Required: it is how every other tool
	// reaches the Airflow this record describes.
	Port int

	// Hostname is the proxy hostname for this project, when the consumer
	// publishes one. Optional, and display-only — state is keyed by project
	// path, never by hostname.
	Hostname string

	// AirflowMajor is the Airflow generation this runtime runs ("2" or "3").
	// Optional but strongly wanted: it is a fact about the process, not about
	// the manifest, so anything talking to this Airflow later follows the
	// process rather than a pin that may since have been edited.
	AirflowMajor string

	// StartedAt is when the supervised Airflow actually started. Zero means
	// "now", which is right for the common case of claiming an Airflow the
	// consumer has just brought up.
	//
	// It is settable because the consumer is the only party that knows. A claim
	// is written once, so without this a consumer re-claiming an Airflow it has
	// been running for hours — after its own restart, or after a sweep removed
	// the record while a probe blipped — would republish it with an age of
	// zero, and `astro local list` would report a six-hour-old process as
	// freshly started.
	StartedAt time.Time
}
