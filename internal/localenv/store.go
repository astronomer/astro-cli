package localenv

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/connmodel"
)

// Kind is what a stored value is: a plain env var, a connection, or an
// Airflow Variable. Each maps to a distinct Airflow env-var name.
type Kind string

const (
	KindEnv  Kind = "env"
	KindConn Kind = "conn"
	KindVar  Kind = "var"
)

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
		return "", invalidName(kind, name)
	}
	if kind == KindConn {
		value, err = normalizeConn(name, value)
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
		return "", false, invalidName(kind, name)
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
		return false, invalidName(kind, name)
	}
	return mergeDelete(s.Path, key)
}

func invalidName(kind Kind, name string) error {
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

// normalizeConn turns a user-supplied connection value — a connection URI or
// the AIRFLOW_CONN_* JSON — into the canonical single-line JSON the codec
// produces, so `check` and `list` can always decode it. It stores credentials
// in plaintext, which is the whole posture of this feature.
func normalizeConn(connID, raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("connection %q: empty value", connID)
	}
	var conn connmodel.Connection
	if strings.HasPrefix(trimmed, "{") {
		decoded, ok := airflowenv.DecodeConnEnv(airflowenv.EnvKeyForConnID(connID), trimmed)
		if !ok {
			return "", fmt.Errorf("connection %q: value is not valid connection JSON", connID)
		}
		conn = decoded
	} else {
		c, err := connFromURI(connID, trimmed)
		if err != nil {
			return "", err
		}
		conn = c
	}
	_, val, ok := airflowenv.EncodeConnEnv(conn)
	if !ok {
		return "", fmt.Errorf("connection %q: could not encode value", connID)
	}
	return val, nil
}

// connFromURI parses an Airflow connection URI into a Connection. The scheme
// is the conn type, userinfo the login/password, host/port the endpoint, the
// path the schema, and the query the extra map.
func connFromURI(connID, uri string) (connmodel.Connection, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme == "" {
		return connmodel.Connection{}, fmt.Errorf("connection %q: value is neither JSON nor a URI (expected conn_type://... or {\"conn_type\":...})", connID)
	}
	conn := connmodel.Connection{
		ConnID:     connID,
		ConnType:   u.Scheme,
		ConnHost:   u.Hostname(),
		ConnSchema: strings.TrimPrefix(u.Path, "/"),
	}
	if p := u.Port(); p != "" {
		port, perr := strconv.Atoi(p)
		if perr != nil {
			return connmodel.Connection{}, fmt.Errorf("connection %q: port %q is not a number", connID, p)
		}
		conn.ConnPort = port
	}
	if u.User != nil {
		conn.ConnLogin = u.User.Username()
		if pw, hasPw := u.User.Password(); hasPw {
			conn.ConnPassword = pw
		}
	}
	if q := u.Query(); len(q) > 0 {
		extra := make(map[string]any, len(q))
		for k, vs := range q {
			if len(vs) == 1 {
				extra[k] = vs[0]
			} else {
				extra[k] = vs
			}
		}
		conn.ConnExtra = extra
	}
	return conn, nil
}
