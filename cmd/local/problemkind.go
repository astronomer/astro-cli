package local

import (
	"errors"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/instancelocate"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// The kinds a v2 command can fail with. The mechanism they plug into — the
// ProblemKind type, the ordered table, what earns a kind — is the whole CLI's
// and lives in cmd/cliout; the root composes this table with its own.
//
// These names are contract. Adding one is cheap; changing one breaks whatever
// was reading it, which is the whole point of having them.
const (
	// KindNoProject: neither this directory nor any above it is an astro
	// project. First for a consumer as well as first in the table — it is the
	// answer to "can I run anything here at all", and every other kind
	// presumes it is not this.
	KindNoProject cliout.ProblemKind = "no_project"
	// KindForeignMode: the project has a runtime record from the other mode,
	// which only that mode's engine can act on.
	KindForeignMode cliout.ProblemKind = "foreign_mode"
	// KindAlreadyRunning: a local Airflow is already live for this project.
	// Not transient; something has to stop first.
	KindAlreadyRunning cliout.ProblemKind = "already_running"
	// KindHealthTimeout: Airflow did not answer in the time allowed. The
	// runtime may still be coming up — the message says what its mode left
	// behind.
	KindHealthTimeout cliout.ProblemKind = "health_timeout"
	// KindLocked: another operation holds this project's lock. The one kind
	// here that says "try again" rather than "something is wrong".
	//
	// Unix only, and not by choice: the Windows lock takes no flock and so
	// never reports contention (localstate/lock_windows.go). A consumer's
	// retry policy is correct everywhere; it simply has nothing to fire on
	// there.
	KindLocked cliout.ProblemKind = "locked"
	// KindUnsupportedBase: the project's declared Dockerfile does not build on
	// an Astro Runtime image, so docker mode refuses before starting anything.
	// Not transient and not about the machine: the project has to change.
	KindUnsupportedBase cliout.ProblemKind = "unsupported_base"
	// KindDatabaseNewerThanAirflow: a newer Airflow already upgraded the
	// project's metadata database, and the Airflow it now runs cannot migrate it
	// back. Not transient: the pin has to go back up, or the database has to go.
	// Docker mode only today.
	KindDatabaseNewerThanAirflow cliout.ProblemKind = "database_newer_than_airflow"
	// KindNotRunning: no local Airflow is running for this project.
	KindNotRunning cliout.ProblemKind = "not_running"
	// KindDeploymentHibernating: the Astro Deployment is hibernating, so its
	// Airflow cannot answer until something wakes it.
	KindDeploymentHibernating cliout.ProblemKind = "deployment_hibernating"
	// KindDeploymentDeploying: the Astro Deployment is being created or
	// updated, and its Airflow has not come up yet. Transient.
	KindDeploymentDeploying cliout.ProblemKind = "deployment_deploying"
	// KindDeploymentUnhealthy: Astro reports the Deployment unhealthy, and its
	// Airflow is not answering.
	KindDeploymentUnhealthy cliout.ProblemKind = "deployment_unhealthy"
	// KindAirflowUnavailable: Astro reports the Deployment healthy, but its
	// Airflow is not answering yet, as after a wake-up or deploy. Transient.
	KindAirflowUnavailable cliout.ProblemKind = "airflow_unavailable"
)

// ProblemKinds maps a v2 failure to the name it publishes under. The root
// composes it with the cloud kinds and hands the result to cliout.Execute.
//
// Order decides (see cliout.Kinds), and is pinned by a test, so tidying this
// table cannot quietly change what a failure publishes.
//
// What earns a kind is that the CLI can recognize the failure reliably — a
// sentinel it wraps, or a type it can assert. NOT that an outside Go program
// could match the same error: the consumer of --output json reads JSON, and
// *project.NotFoundError lives in internal/, unimportable outside this module,
// which is fine and does not weaken the name at all. An earlier version of this
// rule claimed the opposite and was broken by its own first entry.
//
// What does NOT earn a kind is a failure nothing can currently emit.
// ErrImageNotBuilt and ErrNotImplemented were here and are gone: the first is
// returned only by RunInImage, the second only by HotInstall, Sync and
// Airflow.Env, and none of those is on cmd/local's Runtime interface.
var ProblemKinds = cliout.Kinds{
	{Kind: KindNoProject, Match: func(err error) bool {
		var notFound *project.NotFoundError
		var noSection *project.NoAstroSectionError
		return errors.As(err, &notFound) || errors.As(err, &noSection)
	}},
	// Before already_running: a refusal carrying both is the more specific
	// complaint, since the mode is why stopping and starting again will not
	// help.
	{Kind: KindForeignMode, Match: cliout.Sentinel(localrt.ErrForeignMode)},
	{Kind: KindAlreadyRunning, Match: cliout.Sentinel(localrt.ErrAlreadyRunning)},
	{Kind: KindHealthTimeout, Match: cliout.Sentinel(localrt.ErrHealthTimeout)},
	{Kind: KindUnsupportedBase, Match: cliout.Sentinel(localrt.ErrUnsupportedBase)},
	{Kind: KindDatabaseNewerThanAirflow, Match: cliout.Sentinel(localrt.ErrDatabaseNewerThanAirflow)},
	// ErrStartInProgress, not a second name for it: pkg/localrt already
	// exports this sentinel, and adding another public alias to a module with
	// its own go.mod would mean one failure with two contracts.
	{Kind: KindLocked, Match: cliout.Sentinel(localrt.ErrStartInProgress)},
	{Kind: KindNotRunning, Match: cliout.Sentinel(localrt.ErrNotRunning)},
	{Kind: KindDeploymentHibernating, Match: cliout.Sentinel(instancelocate.ErrDeploymentHibernating)},
	{Kind: KindDeploymentDeploying, Match: cliout.Sentinel(instancelocate.ErrDeploymentDeploying)},
	{Kind: KindDeploymentUnhealthy, Match: cliout.Sentinel(instancelocate.ErrDeploymentUnhealthy)},
	{Kind: KindAirflowUnavailable, Match: cliout.Sentinel(instancelocate.ErrAirflowUnavailable)},
}
