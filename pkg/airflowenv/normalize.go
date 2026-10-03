package airflowenv

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/pkg/connmodel"
)

// NormalizeConn turns a connection written by a person — a connection URI, or
// the AIRFLOW_CONN_* JSON — into the canonical single-line JSON this codec
// produces, so whatever reads it back can always decode it.
//
// It is the one definition of what a stored connection looks like, and it lives
// here rather than beside a caller because there are now two: `astro local env
// connection set`, and the conversion that carries a v1 airflow_settings.yaml into
// the vault. Two spellings of "canonical" is how one tool writes a record the
// other cannot read.
func NormalizeConn(connID, raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("connection %q: empty value", connID)
	}
	var conn connmodel.Connection
	if strings.HasPrefix(trimmed, "{") {
		decoded, ok := DecodeConnEnv(EnvKeyForConnID(connID), trimmed)
		if !ok {
			return "", fmt.Errorf("connection %q: value must be connection JSON carrying a conn_type", connID)
		}
		conn = decoded
	} else {
		c, err := ConnFromURI(connID, trimmed)
		if err != nil {
			return "", err
		}
		conn = c
	}
	_, val, ok := EncodeConnEnv(conn)
	if !ok {
		return "", fmt.Errorf("connection %q: could not encode value", connID)
	}
	return val, nil
}

// ConnFromURI parses an Airflow connection URI into a Connection. The scheme is
// the conn type, userinfo the login/password, host/port the endpoint, the path
// the schema, and the query the extra map.
//
// It reads a URI the way Airflow's Connection does, because a URI is usually
// one Airflow wrote or documented: the scheme is normalized (postgresql is
// postgres, a hyphen is an underscore), and a second scheme after the first
// (http://https://api.example.com) is the host's protocol, kept on the host.
//
// Only a second scheme where the host starts is a protocol. One further along
// is part of a path or query and parses as written: an extra holding a URL,
// like aws://?endpoint_url=http://localhost:4566, is common and is not usually
// percent-encoded. Counting "://" across the whole string, as Airflow does,
// would read that as a protocol.
func ConnFromURI(connID, uri string) (connmodel.Connection, error) {
	invalid := fmt.Errorf("connection %q: value is neither JSON nor a URI (expected conn_type://... or {\"conn_type\":...})", connID)
	var protocol string
	if connType, rest, found := strings.Cut(uri, "://"); found {
		if p, host, ok := leadingScheme(rest); ok {
			// A protocol is a scheme name. Anything else there, like userinfo
			// or a port, means the string is malformed, and so does a third
			// scheme on top of the protocol; Airflow refuses both.
			if !schemeName.MatchString(p) {
				return connmodel.Connection{}, invalid
			}
			if _, _, again := leadingScheme(host); again {
				return connmodel.Connection{}, invalid
			}
			protocol = p
			uri = connType + "://" + host
		}
	}
	u, err := url.Parse(uri)
	if err != nil || u.Scheme == "" {
		return connmodel.Connection{}, invalid
	}
	conn := connmodel.Connection{
		ConnID:     connID,
		ConnType:   NormalizeConnType(u.Scheme),
		ConnHost:   u.Hostname(),
		ConnSchema: strings.TrimPrefix(u.Path, "/"),
	}
	if protocol != "" && conn.ConnHost != "" {
		conn.ConnHost = protocol + "://" + conn.ConnHost
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
		// __extra__ is an envelope, not an extra. Airflow's own
		// Connection.get_uri() puts the whole extras object in it, JSON-encoded,
		// so a v1 file holding an exported URI carries its extras there. Reading
		// it as an ordinary parameter stores a connection with one extra named
		// __extra__ and none of the real ones, which fails to connect for a
		// reason nothing names.
		if env, only := q["__extra__"]; only && len(q) == 1 && len(env) == 1 {
			var unwrapped map[string]any
			if err := json.Unmarshal([]byte(env[0]), &unwrapped); err == nil {
				conn.ConnExtra = unwrapped
				return conn, nil
			}
			// Not JSON: fall through and keep it as written rather than drop it.
		}
		extra := make(map[string]any, len(q))
		for k, vs := range q {
			// A repeated parameter keeps only the last value: Airflow's extras
			// are a flat string map, and a list here becomes a JSON array that
			// no provider knows how to read.
			extra[k] = vs[len(vs)-1]
		}
		conn.ConnExtra = extra
	}
	return conn, nil
}

// schemeName is RFC 3986's scheme syntax.
var schemeName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*$`)

// leadingScheme splits "<p>://<after>" off rest when that "://" comes before
// any "/", "?" or "#" of rest's own: where a host would start, rather than
// inside a path, query or fragment. ok is false when there is no such "://".
func leadingScheme(rest string) (p, after string, ok bool) {
	i := strings.Index(rest, "://")
	if i < 0 {
		return "", "", false
	}
	if j := strings.IndexAny(rest, "/?#"); j < i {
		return "", "", false
	}
	return rest[:i], rest[i+len("://"):], true
}

// NormalizeConnType is the conn type Airflow reads from a URI scheme. A scheme
// cannot carry an underscore, so Airflow spells google_cloud_platform as
// google-cloud-platform there, and it accepts SQLAlchemy's postgresql for its
// own postgres.
func NormalizeConnType(scheme string) string {
	if scheme == "postgresql" {
		return "postgres"
	}
	return strings.ReplaceAll(scheme, "-", "_")
}
