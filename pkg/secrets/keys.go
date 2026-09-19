package secrets

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// The key grammar for the shared vault.
//
// The store itself takes an opaque string key, which is the right shape for a
// store and the wrong place to decide what keys mean. This is that decision, and
// it lives here rather than in either consumer because a vault two tools cannot
// address identically is a vault they cannot share — and the failure would be
// silent, each tool reading and writing perfectly well while seeing none of the
// other's values.
//
// A key is three parts: what kind of thing it is, which checkout it belongs to,
// and what it is called.
//
//	env:/Users/me/projects/etl:DATABASE_URL
//	var:global:SLACK_CHANNEL
//	conn:/Users/me/projects/etl:warehouse
//
// Scope is a canonical filesystem path, or the literal GlobalScope. Canonical
// meaning localrt.CanonicalPath's spelling, which the caller applies — see Key.
// It is a checkout path rather than a project root on purpose: a worktree holds
// its own values, and collapsing several worktrees onto one project path would
// merge them — same key, different intended values, last writer wins and the rest
// gone.
//
// What a key addresses differs by kind, and that asymmetry is the domain's rather
// than ours. A connection is a record — type, host, login, port, password — and a
// consumer that can only read its password cannot build a connection from it, so
// the value under a conn key is the whole object. A variable is a scalar, so the
// value under an env or var key is just that: the value. Only secret variables
// are stored here at all; a plain one has nothing to protect and reading it
// should not cost a keyring round trip.
//
// The conn ENCODING is part of the contract, not an implementation detail, and
// naming it here is the cheapest way to keep it one: the value is the single-line
// JSON that Airflow itself parses from an AIRFLOW_CONN_* variable, which
// pkg/airflowenv's codec produces — conn_type at the top with host, login,
// password, schema, port and a nested extra object. This package cannot import
// that codec (a sub-module does not import a sibling), so it can only say so. Two
// tools that address a key identically and serialize its value differently
// reproduce exactly the failure the header warns about, one step further in.

// Kind is what a key addresses.
//
// internal/localenv declares an identical enum — same three values — because it
// predates this package. Whichever consumer wires the vault first should delete
// that one and use this, rather than converting between two enums that are free to
// drift; they are only safe while they agree by coincidence.
type Kind string

const (
	// KindEnv is a secret plain environment variable's value.
	KindEnv Kind = "env"
	// KindVar is a secret Airflow Variable's value.
	KindVar Kind = "var"
	// KindConn is a whole connection, serialized.
	KindConn Kind = "conn"
)

// GlobalScope is the scope for values that belong to no single checkout — the
// desktop's global tier. A literal rather than the empty string so a key always
// has three non-empty parts and a truncated one cannot parse as valid.
const GlobalScope = "global"

// sep divides a key's three parts. One constant because keyring.go embeds the key
// in each on-disk value file AND hashes it into the filename, so a build and a
// parse that disagree about this character do not fail — they orphan every stored
// secret.
const sep = ":"

// ErrBadKey reports a key that cannot be built or parsed.
var ErrBadKey = errors.New("malformed secret key")

// ErrUnknownKind reports a kind this build does not know, wrapping ErrBadKey so
// existing callers keep working.
//
// Distinct because the two tools sharing this vault ship independently. A newer
// peer writing a fourth kind must be skippable by an older reader, which cannot
// happen if "written by a newer build" is indistinguishable from "this entry is
// garbage" — a listing loop would either fail the whole read or silently drop the
// entry, and a cleanup routine that deletes unparseable keys would destroy data
// the peer still owns.
var ErrUnknownKind = fmt.Errorf("%w: unknown kind", ErrBadKey)

// Key builds the vault key for one value.
//
// The caller canonicalizes scope, with localrt.CanonicalPath — the spelling every
// tool agrees on — before calling. This package deliberately does not do it for
// them, and the reason is worth stating because the opposite looks safer: doing it
// here would mean the vault depending on the runtime module, whose signatures are
// declared unstable and whose graph is far larger than a path cleanup is worth. A
// vault coupled to the runtime is a worse trade than a documented precondition.
//
// What is enforced instead is the part that catches a caller who forgot: a scope
// must be GlobalScope or an absolute path. That rejects the realistic mistake — a
// relative path, which would key values under whatever directory the process
// happened to be in — without needing the filesystem. It cannot catch an
// unresolved symlink, so canonicalize.
//
// name is rejected rather than escaped when it contains a colon. Every name this
// grammar carries — an environment variable, an Airflow Variable key, a
// connection id — is already restricted to letters, digits and underscores by the
// code that accepts it, so a colon here means a caller went around that
// validation, and mangling it quietly would produce a key that round-trips to
// something else.
func Key(kind Kind, scope, name string) (string, error) {
	if !kind.valid() {
		return "", fmt.Errorf("%w %q", ErrUnknownKind, kind)
	}
	if err := checkScope(scope); err != nil {
		return "", err
	}
	if name == "" {
		return "", fmt.Errorf("%w: empty name", ErrBadKey)
	}
	if strings.Contains(name, sep) {
		return "", fmt.Errorf("%w: name %q contains %q", ErrBadKey, name, sep)
	}
	return string(kind) + sep + scope + sep + name, nil
}

// valid reports whether this build knows the kind. One method rather than a switch
// per function: the two used to be written out separately, and adding a kind to
// only one of them builds keys the parser rejects with nothing failing — a linter
// would catch a missing enum member, but CI does not lint this module.
func (k Kind) valid() bool {
	switch k {
	case KindEnv, KindVar, KindConn:
		return true
	default:
		return false
	}
}

// A scope is checked on the way in and on the way out, by two different rules, and
// the asymmetry is deliberate.
//
// Writing is strict. checkScope runs on this machine for this machine, so it can
// use filepath and be exact.
//
// Reading is permissive. scopeIsPlausible cannot use filepath, because
// filepath.IsAbs is GOOS-specific: C:\Users\me\etl is not absolute on Unix, so
// validating reads that way would make a legitimate Windows key unreadable by any
// non-Windows build — including this package's own tests, which is how the
// asymmetry was found. In a package whose entire purpose is that two tools read
// the same vault, "the reader rejects what the writer wrote" is a worse failure
// than the one being prevented. So the read side checks the shapes an absolute
// path takes on ANY platform, which is enough for the case that motivated it.
//
// What motivated it: with nothing checked on the read side, ParseKey accepted keys
// Key would never build. "env:global:MY:VAR" parsed to a scope of "global:MY" and
// "env:relative/p:N" to a relative one — so a peer tool writing a name containing
// a colon, the exact skew this package exists to prevent, produced entries a
// reader silently attributed to a checkout that does not exist. A caller filtering
// on GlobalScope would drop the user's global secret; one listing per-project
// secrets would show it under a phantom directory.

// checkScope is the write-side rule.
//
// Clean-stability is checked as well as absoluteness, because absoluteness alone
// admits the likelier mistake. "/p/etl", "/p/etl/", "/p/./etl" and "//p/etl" are
// one directory and four different keys, and on Windows "C:/x" and "C:\x" are
// interchangeable to every Go path API and distinct here. Requiring Clean output
// cannot reject a genuinely canonical scope, since localrt.CanonicalPath is
// filepath.Abs, then EvalSymlinks, then a respelling to the filesystem's own
// capitalization built with filepath.Join — all of which return Clean-stable
// paths. The last of those was added later; if the respelling ever stops
// going through Join, check that it still cannot emit an unclean path.
//
// What it still cannot catch is an unresolved symlink, which is why canonicalizing
// remains the caller's documented job rather than a promise made here.
func checkScope(scope string) error {
	switch {
	case scope == "":
		return fmt.Errorf("%w: empty scope; use GlobalScope", ErrBadKey)
	case scope == GlobalScope:
		return nil
	case !filepath.IsAbs(scope):
		return fmt.Errorf("%w: scope %q is not absolute; canonicalize it first", ErrBadKey, scope)
	case filepath.Clean(scope) != scope:
		return fmt.Errorf("%w: scope %q is not clean (want %q); canonicalize it first", ErrBadKey, scope, filepath.Clean(scope))
	}
	return nil
}

// ParseKey splits a key back into its parts.
//
// The split is deliberately lopsided: kind runs to the FIRST colon and name from
// the LAST, with everything between them the scope. That is not stylistic — a
// Windows scope is a path like C:\Users\me\etl, so a scope may itself contain
// colons and a left-to-right split would tear it. Names cannot contain one (see
// Key), which is what makes reading from both ends unambiguous.
func ParseKey(key string) (kind Kind, scope, name string, err error) {
	kindStr, rest, found := strings.Cut(key, sep)
	if !found {
		return "", "", "", fmt.Errorf("%w: %q needs kind%sscope%sname", ErrBadKey, key, sep, sep)
	}
	if !Kind(kindStr).valid() {
		return "", "", "", fmt.Errorf("%w %q in %q", ErrUnknownKind, kindStr, key)
	}
	i := strings.LastIndex(rest, sep)
	if i < 0 {
		return "", "", "", fmt.Errorf("%w: %q needs kind%sscope%sname", ErrBadKey, key, sep, sep)
	}
	scope, name = rest[:i], rest[i+1:]
	if name == "" {
		return "", "", "", fmt.Errorf("%w: %q has an empty name", ErrBadKey, key)
	}
	if scope == "" {
		return "", "", "", fmt.Errorf("%w: %q has an empty scope", ErrBadKey, key)
	}
	if !scopeIsPlausible(scope) {
		return "", "", "", fmt.Errorf("%w: scope %q in %q is neither %q nor an absolute path", ErrBadKey, scope, key, GlobalScope)
	}
	return Kind(kindStr), scope, name, nil
}

// scopeIsPlausible is the read-side rule: GlobalScope, or something shaped like an
// absolute path on some platform. Hand-rolled rather than filepath.IsAbs so that a
// key stays readable off the platform that wrote it — see the note above
// checkScope for why that matters more here than exactness does.
func scopeIsPlausible(scope string) bool {
	switch {
	case scope == GlobalScope:
		return true
	case strings.HasPrefix(scope, "/"), strings.HasPrefix(scope, `\\`):
		return true // POSIX, or a Windows UNC share
	case len(scope) >= 3 && isDriveLetter(scope[0]) && scope[1] == ':' && (scope[2] == '\\' || scope[2] == '/'):
		return true // C:\... or C:/...
	}
	return false
}

func isDriveLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
