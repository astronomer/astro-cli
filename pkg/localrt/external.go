package localrt

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// ExternalRuntime is the contract for a runtime the caller supervises itself.
// See rt.ExternalRuntime, and Reserve below for how a claim is made.
type ExternalRuntime = rt.ExternalRuntime

// The errors a consumer has to be able to tell apart. They are sentinels
// because the alternative is substring-matching a message, and the right
// response differs sharply between them: retry shortly, refuse and surface,
// or stop supervising.
//
// The rule behind them is Start's, made stricter. Start refuses a start over a
// live runtime and so does a claim, which is the part that must not diverge —
// two tools disagreeing about when a start is safe would only show up in
// cross-tool use. Where refuseClaim goes further is documented on it: a foreign
// mode is refused whether or not it probes live, because a claim overwriting a
// docker record strands the compose stack it describes.
//
// The messages are deliberately tool-neutral even where the rule is shared.
// Start's says "use `astro local restart`", which is the wrong instruction to
// put in front of a consumer's user, so a claim reports a typed error and lets
// the consumer say what its own user should do.
var (
	// ErrStartInProgress means another start holds the project's lock right
	// now. Transient: the holder either finishes or dies, and its lock goes
	// with it.
	ErrStartInProgress = localstate.ErrLocked

	// ErrAlreadyRunning means a local Airflow is already live for this project.
	// Not transient — something has to stop first.
	ErrAlreadyRunning = errors.New("a local Airflow is already running for this project")

	// ErrForeignMode means the project has a record from the other runtime
	// mode. Returned whether or not that runtime probes as live, because the
	// record carries state only its own engine can act on and overwriting it
	// would strand what it describes.
	ErrForeignMode = errors.New("this project has a runtime record from a different mode")

	// ErrGroupNotAlive means the claim named a process group that is not
	// running, so the record would have read as stopped the moment it landed.
	ErrGroupNotAlive = errors.New("the process group named by this claim is not alive")

	// ErrNotOurs means the record for this project describes some other
	// runtime, so the caller's request to drop it was declined.
	ErrNotOurs = errors.New("the runtime record for this project belongs to another runtime")

	// ErrClaimUnsupported means standalone claims cannot work on this platform.
	// Only Windows returns it — see claimSupported there for why — but it is
	// declared on every platform so a consumer's errors.Is against it compiles
	// everywhere rather than only where it can fire.
	ErrClaimUnsupported = errors.New("claiming a standalone runtime is not supported on this platform")
)

// Reservation is a held start lock for one project, taken only once nothing else
// is running there: while it is open, this consumer is the one bringing an
// Airflow up.
//
// It exists because of the gap between "a consumer decides to start Airflow" and
// "there is an Airflow process group to record", which for a real provisioning
// run is minutes — a venv sync, an image pull, a health wait — and is exactly
// when a user who sees nothing happening reaches for the other tool. A record
// cannot cover that window: a record needs a live process group, and the only
// group available before Airflow exists is the consumer's own, which a stop
// would then SIGKILL (see rt.ExternalRuntime.Pgid).
//
// The lock covers it instead. A competing `astro local start` fails fast on it
// with ErrStartInProgress, which is both true and the same thing that happens
// when two CLI starts race.
//
// Claim is a method here rather than on Runtime so the check-and-write cannot be
// performed without the lock. That is the invariant this type protects, and
// making it structural means no caller can forget it.
type Reservation struct {
	rt   *Runtime
	path string

	// mu guards unlock, which Close clears. The intended consumer is a
	// long-lived supervisor whose idle-cool timer and provisioning goroutine
	// can reach a reservation at the same time, so an unsynchronized field
	// would let two Closes double-release the same descriptor, or let a Claim
	// write the record just after the lock was dropped — losing the
	// check-and-write-under-lock guarantee this type is documented to give.
	mu     sync.Mutex
	unlock func()
}

// Reserve refuses if a local Airflow is already running for the project, then
// takes its start lock and holds it until the Reservation is closed. Call it
// before starting to bring an Airflow up, and close it once the claim is written
// or the attempt has failed.
//
// The refusal happens HERE, before the caller provisions anything, and that
// placement is the whole point rather than an optimization. An earlier version
// of this API checked only at Claim, which runs after provisioning: a project
// the CLI was already running would let a consumer sync a venv and spawn its own
// Airflow against the same project directory and metadata database, and learn it
// was the second one only at the end. Two live Airflows, the newer unrecorded
// and so invisible to `astro local stop`, which is the exact failure this whole
// API exists to prevent. A consumer cannot make the check itself without a
// window between looking and locking; Reserve is where it is atomic.
//
// The lock is an flock on the project's state dir, so it is released if the
// holder dies. A consumer that crashes mid-start leaves nothing wedged, which is
// why this is a lock rather than a record with a "starting" state.
func (r *Runtime) Reserve(projectPath string) (*Reservation, error) {
	path, err := externalPath(projectPath)
	if err != nil {
		return nil, err
	}
	// Before taking the lock, because on Windows the lock does not lock (see
	// claimSupported there) and a reservation that cannot exclude anything
	// should not be handed out at all.
	if err := claimSupported(); err != nil {
		return nil, err
	}
	unlock, err := localstate.Lock(path)
	if err != nil {
		return nil, err
	}
	if err := r.refuseClaim(path, ModeStandalone); err != nil {
		unlock()
		return nil, err
	}
	return &Reservation{rt: r, path: path, unlock: unlock}, nil
}

// Close releases the lock. Safe to call more than once and from more than one
// goroutine, so `defer res.Close()` composes with an explicit close on the
// success path.
func (res *Reservation) Close() {
	res.mu.Lock()
	defer res.mu.Unlock()
	if res.unlock == nil {
		return
	}
	res.unlock()
	res.unlock = nil
}

// Claim records the runtime the caller has brought up, under the reservation's
// lock, so the rest of the toolchain can see it and nothing else starts a second
// Airflow over it.
//
// One record, written once, when the consumer has all the facts. There is no
// update call: the earlier shape of this API let a claim be written first and
// corrected later, which needed an ownership test it could not actually perform
// — for standalone records the only thing distinguishing "my claim" from
// "someone else's live runtime" was the mode, which is the same for both — and a
// partial correction silently erased the hostname and generation the first write
// published. Reserve removes the reason for it: hold the lock while
// provisioning, then write the record once Airflow is up.
//
// A restart is a Release and a fresh Reserve/Claim, not an update, because a
// restarted Airflow is a different process group with a different start time.
//
// The guard runs again here even though Reserve already ran it and holds the
// lock. It is a disk read against a file nothing else may write right now, so it
// costs nothing, and it means Claim states its own precondition rather than
// inheriting it from a caller that may have been refactored.
//
// The record deliberately does not set StopWithSession. That field ties an
// Airflow's lifetime to the process that started it, and a claim's lifetime is
// the consumer's business, governed by its own supervisor rather than by any
// command's exit.
func (res *Reservation) Claim(ext ExternalRuntime) error {
	res.mu.Lock()
	defer res.mu.Unlock()
	if res.unlock == nil {
		return errors.New("this reservation is closed; take a new one before claiming")
	}
	rec, err := res.rt.externalRecord(ext)
	if err != nil {
		return err
	}
	// Compared by project identity, not by string. The reservation's lock and
	// the record's location are both keyed by the resolved path's hash, so two
	// spellings that hash the same are genuinely the same reservation — and
	// refusing one of them here would fail a claim the lock provably covered,
	// after provisioning had already finished.
	same, err := sameProject(rec.ProjectPath, res.path)
	if err != nil {
		return err
	}
	if !same {
		return fmt.Errorf("this reservation is for %s, not %s", res.path, rec.ProjectPath)
	}
	if err := res.rt.refuseClaim(rec.ProjectPath, rec.Mode); err != nil {
		return err
	}
	return localstate.Save(rec)
}

// refuseClaim is the live-start guard for a claim. Same rule as Start's, with
// two differences that both matter.
//
// It checks the MODE BEFORE asking whether the runtime is live, and refuses a
// foreign mode either way. Start's guard only refuses a live one, which is right
// for it — a dead same-mode record is stale state its own engine may overwrite —
// but wrong here: a docker record whose daemon is merely stopped probes as
// stopped, and letting a standalone claim overwrite it drops the compose project
// name, after which nothing can find or tear that stack down. The engines' own
// checkNotRunning refuses a foreign mode unconditionally; this matches that
// rather than the weaker rule.
//
// Ordering the mode check first also keeps this path off the container engine
// entirely. Asking a docker record whether it is live shells out to
// `docker compose ls` with no deadline, and doing that under the project's lock
// would let a wedged daemon block a consumer indefinitely with nothing to cancel
// it. Refusing on the mode alone is pure disk, so the question never gets asked.
func (r *Runtime) refuseClaim(projectPath string, mode Mode) error {
	prev, err := localstate.Load(projectPath)
	if errors.Is(err, localstate.ErrNotRunning) {
		return nil
	}
	if err != nil {
		return err
	}
	if recordMode(prev) != mode {
		return fmt.Errorf("%w (%s); stop it before starting another", ErrForeignMode, modeLabel(prev.Mode))
	}
	if r.statusOf(prev).State == StateRunning {
		return ErrAlreadyRunning
	}
	return nil
}

// recordMode is a record's mode with the empty value resolved, matching what
// every other reader of a record does with it: localprune treats empty as
// standalone by name, and Runtime.statusOf routes it to the standalone engine.
//
// Without this the mode comparison rejected a record written before the field
// existed as "foreign", and rendered it as "a runtime record from a different
// mode (standalone)" against a standalone claim — a message that names the
// claim's own mode as the alien one, for a project that would then be
// permanently unclaimable.
func recordMode(rec localstate.Record) Mode {
	if rec.Mode == "" {
		return ModeStandalone
	}
	return rec.Mode
}

// Release drops the record for a claim, so the project reads as stopped
// everywhere. Removing an absent record is not an error, which keeps a
// consumer's teardown idempotent the way the engines' own stop paths are.
//
// pid identifies the claim being released, and anything else is left alone with
// ErrNotOurs. Without that check a consumer's teardown could delete a record it
// no longer owns: its Airflow dies, the user runs `astro local start`, and the
// consumer's idle timer then removes the record for the CLI's live Airflow —
// orphaning it from `astro local list` and from `astro local stop`, which would
// have no record left to attach to.
//
// A non-positive pid is refused rather than treated as a wildcard. Docker
// records omit PID entirely, so `Release(project, 0)` would have matched every
// one of them and deleted it, losing the compose project name that is the only
// handle on that stack — the very outcome ErrForeignMode exists to prevent on
// the way in. The mode is checked for the same reason: a claim never writes a
// docker record, so finding one means this is not our project.
//
// The consumer supplies the pid because only it knows what it claimed, and the
// exported readers deliberately do not report a stopped runtime's pid (see
// Record.Status). A supervisor that must survive its own restart should persist
// its claim the way it persists anything else about a project it manages; where
// that is genuinely lost, PruneStale is the machine-wide sweep for it.
//
// The lock is taken when it is free and skipped only when another start holds
// it, since a teardown that failed with "another start is in progress" would be
// worse than the small race that leaves. Any other lock failure is reported: it
// means the state directory itself is unusable, and proceeding unlocked would
// silently discard the atomicity this is only meant to trade away for
// contention.
//
// A project directory that no longer exists cannot be released, because the
// state dir is keyed by the resolved path. That leaks a record until PruneStale
// sweeps it, which is what PruneStale is for; release before deleting a project
// where the ordering is yours to choose.
func (r *Runtime) Release(projectPath string, pid int) error {
	if pid <= 0 {
		return fmt.Errorf("releasing a claim needs the pid it was claimed with, got %d", pid)
	}
	path, err := externalPath(projectPath)
	if err != nil {
		return err
	}
	// Peeked before locking, because taking the lock creates the project's
	// state directory: a teardown for a project that has no runtime state
	// should not leave one behind for localstate.List to walk forever.
	if _, err := localstate.Load(path); err != nil {
		if errors.Is(err, localstate.ErrNotRunning) {
			return nil
		}
		return err
	}
	unlock, lockErr := localstate.Lock(path)
	switch {
	case lockErr == nil:
		defer unlock()
	case errors.Is(lockErr, localstate.ErrLocked):
		// Proceed unlocked; see the doc comment.
	default:
		return lockErr
	}
	prev, err := localstate.Load(path)
	if errors.Is(err, localstate.ErrNotRunning) {
		return nil
	}
	if err != nil {
		return err
	}
	if recordMode(prev) != ModeStandalone {
		return fmt.Errorf("%w (%s)", ErrNotOurs, modeLabel(prev.Mode))
	}
	if prev.PID != pid {
		return fmt.Errorf("%w (its pid is %d, not %d)", ErrNotOurs, prev.PID, pid)
	}
	return localstate.Remove(path)
}

// externalRecord validates a claim and renders it as a record.
//
// Every check here guards a failure that is silent rather than loud. A zero PID
// or port publishes a record that misdescribes a healthy Airflow, pointing
// readers at a port nothing listens on or reading as dead. A group that is not
// alive is worse, because the record then reads as stopped from the moment it
// lands: the project drops out of `astro local list` and a competing start stops
// refusing, which is the whole failure this API exists to prevent. Refusing the
// claim is the only outcome that tells the consumer something is wrong while it
// can still do something about it.
func (r *Runtime) externalRecord(ext ExternalRuntime) (localstate.Record, error) {
	if ext.Mode == "" {
		ext.Mode = ModeStandalone
	}
	switch ext.Mode {
	case ModeStandalone:
	case ModeDocker:
		return localstate.Record{}, errors.New("docker mode runs through this package's own engine, which records itself; use Start rather than claiming it externally")
	default:
		return localstate.Record{}, fmt.Errorf("unknown runtime mode %q", ext.Mode)
	}
	if err := claimSupported(); err != nil {
		return localstate.Record{}, err
	}
	if ext.PID <= 0 {
		return localstate.Record{}, errors.New("a claim needs the pid of the process it supervises")
	}
	if ext.Port <= 0 {
		return localstate.Record{}, errors.New("a claim needs the port Airflow listens on")
	}
	path, err := externalPath(ext.ProjectPath)
	if err != nil {
		return localstate.Record{}, err
	}
	startedAt := ext.StartedAt
	if startedAt.IsZero() {
		startedAt = r.now()
	}
	rec := localstate.Record{
		ProjectPath:  path,
		Mode:         ext.Mode,
		PID:          ext.PID,
		Port:         ext.Port,
		Hostname:     ext.Hostname,
		AirflowMajor: ext.AirflowMajor,
		StartedAt:    startedAt.UTC(),
	}
	// Resolved through the record's own accessor and written out, so the record
	// on disk says outright which group it means instead of leaving every reader
	// to infer it. GroupID also declines to salvage a negative pgid — a consumer
	// that had already negated for kill(-pgid, 0) would otherwise store -1234,
	// which every liveness probe reads as dead — so the guard below is what
	// turns that into an error the consumer sees.
	rec.Pgid = localstate.Record{Pgid: ext.Pgid, PID: ext.PID}.GroupID()
	if rec.Pgid <= 0 {
		return localstate.Record{}, fmt.Errorf("%w: %d is not a process group", ErrGroupNotAlive, ext.Pgid)
	}
	// Asked through the same probe every reader uses, so a claim that would have
	// read as stopped is refused instead of published.
	if r.statusOf(rec).State != StateRunning {
		return localstate.Record{}, fmt.Errorf("%w: pid %d, group %d", ErrGroupNotAlive, rec.PID, rec.Pgid)
	}
	return rec, nil
}

// externalPath is how a claim spells a project directory: absolute, symlinks
// left alone.
//
// It matches what the engines store rather than resolving symlinks, and the
// difference is load-bearing. internal/project makes a project's Dir absolute
// and no more, so no engine-written record is ever symlink-resolved, and
// Runtime.List joins records to routes.json by comparing ProjectPath against a
// route's ProjectDir as plain strings. A claim that stored the resolved spelling
// would miss that join for any project under a symlink — a linked worktree,
// anything below /tmp on macOS — and List would then report an empty hostname,
// which in turn makes PruneStale skip removing the route and leak it.
//
// The state directory is keyed by the resolved path's hash either way, so this
// choice cannot split state; it only decides whether the stored string compares
// equal to the one the rest of the toolchain is holding.
//
// This rule is spelled out here and, separately, in localstandalone's Start and
// LogHandle. It is deliberately NOT centralized the way Record.GroupID was,
// because those two wrap the failure in their own messages and unifying them
// would change engine error text for no behavioral gain. The cost is that a
// future change to the rule — the engines adopting filepath.Clean, say — has to
// land in all three or it splits claimed records from engine records and
// silently breaks Runtime.List's routes.json join. Change them together.
func externalPath(projectPath string) (string, error) {
	if projectPath == "" {
		return "", errors.New("a claim needs a project path")
	}
	abs, err := filepath.Abs(projectPath)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", projectPath, err)
	}
	return abs, nil
}

// sameProject reports whether two spellings name the same project, by the same
// identity every piece of per-project state is keyed on.
func sameProject(a, b string) (bool, error) {
	ida, err := ProjectID(a)
	if err != nil {
		return false, err
	}
	idb, err := ProjectID(b)
	if err != nil {
		return false, err
	}
	return ida == idb, nil
}
