package localenv

import (
	"fmt"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
)

// Kind is what a stored value is: a plain env var, a connection, or an
// Airflow Variable. Each maps to a distinct Airflow env-var name.
type Kind string

const (
	KindEnv  Kind = "env"
	KindConn Kind = "conn"
	KindVar  Kind = "var"
)

// Noun is the `astro local env` subcommand that manages a kind: the word
// `astro env` uses for the same object on the cloud side, so `connection` and
// `airflow-variable` mean one thing across both trees.
//
// It lives here because every hint that tells a user what to run composes the
// command from it. Spelled at each call site instead, a renamed subcommand
// leaves the hints naming a command that no longer exists — which is a class
// of bug nothing fails on, because a hint is a string.
func Noun(kind Kind) string {
	switch kind {
	case KindConn:
		return "connection"
	case KindVar:
		return "airflow-variable"
	case KindEnv:
		return "variable"
	default:
		// Not "variable". An unmapped kind here would otherwise compose a
		// runnable command that writes the wrong kind, which is the failure
		// this function exists to prevent; an empty noun makes the hint
		// visibly broken instead.
		return ""
	}
}

// Scope labels the file a value lives in.
type Scope string

const (
	ScopeProject Scope = "project"
	ScopeGlobal  Scope = "global"
)

// Store is one dotenv file with its scope label. Build it with
// ProjectStore or GlobalStore.
type Store struct {
	Path  string
	Scope Scope
}

// ProjectStore is the store for a project's <project>/.env.
func ProjectStore(projectDir string) *Store {
	return &Store{Path: ProjectEnvPath(projectDir), Scope: ScopeProject}
}

// GlobalStore is the store for the machine-wide ~/.astro/env.
func GlobalStore() (*Store, error) {
	path, err := GlobalEnvPath()
	if err != nil {
		return nil, err
	}
	return &Store{Path: path, Scope: ScopeGlobal}, nil
}

// Set writes value for the (kind, name) pair, merge-preserving the rest of
// the file, and returns the Airflow env-var key it wrote under. A connection
// value is normalized to the AIRFLOW_CONN_* JSON form (a URI or JSON in);
// env vars and Variables are stored raw.
func (s *Store) Set(kind Kind, name, value string) (envKey string, err error) {
	key, ok := EnvKeyFor(kind, name)
	if !ok {
		return "", InvalidName(kind, name)
	}
	if kind == KindConn {
		value, err = NormalizeConn(name, value)
		if err != nil {
			return "", err
		}
	}
	if err := mergeSet(s.Path, key, value); err != nil {
		return "", err
	}
	return key, nil
}

// Get returns the stored value for (kind, name) and whether it is present.
// A connection returns its stored JSON form.
func (s *Store) Get(kind Kind, name string) (value string, ok bool, err error) {
	key, valid := EnvKeyFor(kind, name)
	if !valid {
		return "", false, InvalidName(kind, name)
	}
	m, err := readMap(s.Path)
	if err != nil {
		return "", false, err
	}
	v, ok := m[key]
	return v, ok, nil
}

// Delete removes (kind, name) from the file. ok is false when it held no
// such value.
func (s *Store) Delete(kind Kind, name string) (ok bool, err error) {
	key, valid := EnvKeyFor(kind, name)
	if !valid {
		return false, InvalidName(kind, name)
	}
	return mergeDelete(s.Path, key)
}

// InvalidName is the per-kind rejection message, named as a rule the user can
// act on rather than a generic "invalid". Exported so the vault writer reports a
// bad name identically: --secret is meant to change where a value goes and
// nothing else about what the command says.
func InvalidName(kind Kind, name string) error {
	switch kind {
	case KindEnv:
		return fmt.Errorf("%q is not a valid env var name (letters, digits, _; no leading digit)", name)
	case KindVar:
		return fmt.Errorf("%q is not a valid variable key (letters, digits, _)", name)
	case KindConn:
		return fmt.Errorf("%q is not a valid connection id (letters, digits, _)", name)
	default:
		return fmt.Errorf("unknown kind %q", kind)
	}
}

// NormalizeConn turns a user-supplied connection value — a connection URI or
// the AIRFLOW_CONN_* JSON — into the canonical single-line JSON the codec
// produces, so `check` and `list` can always decode it. It stores credentials
// in plaintext, which is the whole posture of this feature.
//
// The rule itself lives in pkg/airflowenv, beside the codec whose output it
// names as canonical, because the conversion that carries a v1
// airflow_settings.yaml into the vault has to write the same shape and cannot
// reach an internal package.
func NormalizeConn(connID, raw string) (string, error) {
	return airflowenv.NormalizeConn(connID, raw)
}
