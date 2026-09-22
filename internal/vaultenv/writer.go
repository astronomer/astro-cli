package vaultenv

import (
	"errors"
	"fmt"

	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

// Writer edits one scope of the shared vault: the store behind
// `astro local env <noun> set --secret`. Build it with NewWriter.
//
// It deliberately mirrors localenv.Store — same Set/Get/Delete shape, same
// (kind, name) addressing, same normalization for a connection — because the
// two are the same operation against different storage, and `--secret` is the
// flag that chooses which. A connection normalized differently by store would
// make one conn_id mean two things depending on where it was written.
type Writer struct {
	store secrets.Store
	// scope is the vault key's scope half: a canonical project path, or
	// secrets.GlobalScope.
	scope string
	// label names this scope the way localenv.Store.Scope does, for the message
	// a set or delete prints.
	label string
}

// NewWriter opens the shared vault for writing. An empty projectDir writes the
// machine-wide scope; otherwise the project's own, keyed by the canonical path
// of its directory so the desktop and this tool agree on the key.
//
// Opening does not touch the keyring: pkg/secrets acquires the master key on the
// first read or write. So this succeeds on a machine with no reachable keyring,
// and the refusal comes from the operation, where it can name what failed.
func NewWriter(projectDir string) (*Writer, error) {
	dir, err := secrets.DefaultDir()
	if err != nil {
		return nil, err
	}
	store, err := secrets.NewKeyringStore(secrets.Config{Service: secrets.DefaultService, Dir: dir})
	if err != nil {
		return nil, fmt.Errorf("open the shared vault: %w", err)
	}
	w := &Writer{store: store, scope: secrets.GlobalScope, label: SourceGlobal}
	if projectDir != "" {
		canonical, cerr := localrt.CanonicalPath(projectDir)
		if cerr != nil {
			return nil, fmt.Errorf("resolve the project directory for a vault key: %w", cerr)
		}
		w.scope, w.label = canonical, SourceProject
	}
	return w, nil
}

// ScopeName names the TIER being written, in the same vocabulary the resolution
// chain reports — "vault" or "vault (global)", not "project"/"global".
//
// Deliberately not localenv.Store's vocabulary, even though the two are
// otherwise mirrored. A `set --secret` that reported "project" was
// byte-identical to a plain `set`, so nothing reading the output — a script, an
// agent, a user — could tell whether a credential had landed in the encrypted
// vault or in a committable .env. That is the one distinction the flag exists to
// make, so it belongs in what the command says it did.
func (w *Writer) ScopeName() localenv.Scope { return localenv.Scope(w.label) }

// DotenvPath is empty: the vault writes nothing into the project, so there is no
// plaintext file for a gitignore advisory to be about.
func (w *Writer) DotenvPath() string { return "" }

// Location is where the value lives, for the same message. Not a file path: the
// vault holds one file per key under a name derived from a hash, and pointing a
// user at it would invite hand-editing something only the master key can read.
func (w *Writer) Location() string { return "the vault shared with Astro Desktop" }

// Set stores value for the (kind, name) pair and returns the Airflow env-var
// key it will resolve under.
func (w *Writer) Set(kind localenv.Kind, name, value string) (envKey string, err error) {
	key, vaultKey, err := w.keys(kind, name)
	if err != nil {
		return "", err
	}
	if kind == localenv.KindConn {
		value, err = localenv.NormalizeConn(name, value)
		if err != nil {
			return "", err
		}
	}
	if err := w.store.Set(vaultKey, value); err != nil {
		return "", refusal(err)
	}
	return key, nil
}

// Get returns the stored value for (kind, name) and whether this scope holds
// one.
func (w *Writer) Get(kind localenv.Kind, name string) (value string, ok bool, err error) {
	_, vaultKey, err := w.keys(kind, name)
	if err != nil {
		return "", false, err
	}
	v, err := w.store.Get(vaultKey)
	switch {
	case err == nil:
		return v, true, nil
	case errors.Is(err, secrets.ErrNotFound):
		return "", false, nil
	default:
		return "", false, refusal(err)
	}
}

// Delete removes (kind, name) from this scope. ok is false when it held no such
// value, which is not an error — the same contract localenv.Store.Delete has.
func (w *Writer) Delete(kind localenv.Kind, name string) (ok bool, err error) {
	_, vaultKey, err := w.keys(kind, name)
	if err != nil {
		return false, err
	}
	switch err := w.store.Delete(vaultKey); {
	case err == nil:
		return true, nil
	case errors.Is(err, secrets.ErrNotFound):
		return false, nil
	default:
		return false, refusal(err)
	}
}

// keys returns both names one operation needs: the Airflow env-var key the value
// will resolve under, and the vault key it is stored at.
func (w *Writer) keys(kind localenv.Kind, name string) (envKey, vaultKey string, err error) {
	envKey, ok := localenv.EnvKeyFor(kind, name)
	if !ok {
		return "", "", localenv.InvalidName(kind, name)
	}
	vaultKey, err = secrets.Key(vaultKind(kind), w.scope, name)
	if err != nil {
		return "", "", err
	}
	return envKey, vaultKey, nil
}

// vaultKind maps the local env taxonomy onto the vault grammar's.
//
// The two are the same three things spelled twice — localenv.Kind for the files,
// secrets.Kind for the vault — and this is the one place that says so, rather
// than each caller re-deciding. Collapsing them into one type is the better fix
// and a wider change than this.
func vaultKind(kind localenv.Kind) secrets.Kind {
	switch kind {
	case localenv.KindEnv:
		return secrets.KindEnv
	case localenv.KindConn:
		return secrets.KindConn
	case localenv.KindVar:
		return secrets.KindVar
	default:
		// Not reachable through the commands, which build a Kind from a fixed
		// subcommand. An empty kind fails secrets.Key rather than guessing.
		return ""
	}
}

// refusal turns a vault failure into the answer the user needs — which means
// telling the three conditions apart, because their remediations have nothing in
// common. pkg/secrets separates them for exactly this reason, and says so:
// "Saying 'os keyring unavailable' alone would send someone to check a daemon
// that is running fine."
//
// The order matters: both specific sentinels wrap ErrKeyringUnavailable, so the
// umbrella has to be tested last or it swallows them.
func refusal(err error) error {
	switch {
	case errors.Is(err, secrets.ErrVaultOrphaned):
		// The keyring is fine and the values are intact; the key that decrypts
		// them is gone. Unlocking anything will never help, and pkg/secrets
		// refuses to mint a replacement precisely so this stays legible rather
		// than becoming twenty authentication errors.
		return fmt.Errorf("this machine has encrypted values but the master key that decrypts them is gone: %w\n\n"+
			"A keychain reset, a new login keychain, or a recreated keyring does this. The values cannot be "+
			"recovered. Deleting the vault directory (~/.astro/secrets) clears the condition and loses those "+
			"secrets, which is why this tool will not do it for you", err)
	case errors.Is(err, secrets.ErrMasterKeyUnusable):
		return fmt.Errorf("this machine's stored master key is not a usable key: %w\n\n"+
			"Nothing about the machine is wrong, so there is no daemon to check. Recovering means accepting that "+
			"anything encrypted under the real key is unreadable", err)
	case errors.Is(err, secrets.ErrKeyringUnavailable):
		// The umbrella: no keyring to reach at all.
		return fmt.Errorf("this machine's keyring is unreachable, so a secret cannot be stored or read here: %w\n\n"+
			"On a headless machine or in CI there is no keyring to hold the master key. Supply the value in the "+
			"environment instead, which the resolution chain reads first, or set it without --secret to keep it in "+
			"a plain file", err)
	default:
		return err
	}
}

// SetSecret stores a converted value under this writer's scope, which is what
// pkg/scaffold's SecretWriter asks for.
//
// It takes the vault's own kind rather than localenv's, because the caller is a
// conversion reading a v1 file and not a command parsing a flag — there is no
// localenv.Kind anywhere in that path, and translating one into the other just
// to translate it back is a round trip through a vocabulary neither side uses.
//
// Set's normalization is deliberately not repeated: the conversion writes
// values that already came out of airflowenv, which is the canonical form Set
// normalizes TO. Re-normalizing would decode and re-encode a value this process
// just encoded.
func (w *Writer) SetSecret(kind secrets.Kind, name, value string) error {
	key, err := secrets.Key(kind, w.scope, name)
	if err != nil {
		return err
	}
	if err := w.store.Set(key, value); err != nil {
		return refusal(err)
	}
	return nil
}

// HasSecret reports whether this scope already holds a value for (kind, name),
// which is what pkg/scaffold's SecretWriter asks so a conversion can decline to
// overwrite a credential the user set deliberately.
func (w *Writer) HasSecret(kind secrets.Kind, name string) (bool, error) {
	key, err := secrets.Key(kind, w.scope, name)
	if err != nil {
		return false, err
	}
	if _, err := w.store.Get(key); err != nil {
		if errors.Is(err, secrets.ErrNotFound) {
			return false, nil
		}
		return false, refusal(err)
	}
	return true, nil
}
