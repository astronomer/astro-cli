// Package vaultenv resolves local env values from the encrypted vault this
// machine shares with Astro Desktop (pkg/secrets). It is the third kind of
// source in the local chain, beside the plain files (internal/localenv) and the
// workspace's Environment Manager (internal/emenv), and it exists so a value
// set in either tool is readable in the other.
//
// Two tiers, keyed by the grammar pkg/secrets defines: the project's own
// secrets under the canonical path of its directory, and the machine-wide ones
// under the literal global scope. They enter the chain at different positions
// (see Providers), which is what makes a project's secret beat a global one of
// the same name.
//
// # Why the index is built forwards
//
// A Provider is asked about an Airflow env-var key (FOO, AIRFLOW_VAR_FOO,
// AIRFLOW_CONN_FOO); the vault is keyed on the bare name. Deriving the vault
// key from the env key looks like the obvious implementation and is wrong,
// because the encoding is lossy: airflowenv upper-cases, so the Variables
// "my_var" and "MY_VAR" both encode to AIRFLOW_VAR_MY_VAR and the stores
// deliberately let both exist. Going backwards would silently miss a
// lower-case name.
//
// So this walks the vault's own listing and encodes each key FORWARDS through
// the same airflowenv functions that produced the env var in the first place.
//
// A collision resolves on the NAME, sorted, last one winning — the rule Astro
// Desktop's spawn overlay already uses, the point being that both tools pick the
// same winner rather than each picking its own. On the name specifically, not on
// the vault key: a key is "kind:scope:name", so sorting keys would let the kind
// token decide and the two tools would disagree for a var and an env var that
// encode to one env-var name.
//
// # Cost
//
// Building the index touches no keyring: pkg/secrets holds each key in
// plaintext inside its value file and decrypts only on Get, and constructing
// the store is lazy. So a run that resolves nothing from the vault never opens
// the keychain, and one that resolves a name opens it once for the run. Nothing
// here is written to disk.
package vaultenv

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/astronomer/astro-cli/internal/envresolve"
	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

// Source labels — the values the `source` field carries in list/get output.
// Distinct per tier because "which one held it" is the question a user asks
// when a project value shadows a global one.
const (
	SourceProject = "vault"
	SourceGlobal  = "vault (global)"
)

// IsVaultSource reports whether a resolved source label names one of these
// tiers. Callers deciding precedence between the vault and a file need this, and
// comparing the label strings by hand at each call site is how one of them comes
// to disagree with the chain.
func IsVaultSource(source string) bool {
	return source == SourceProject || source == SourceGlobal
}

// Source is the vault as one invocation sees it: one store, one listing, two
// tiers. Build it with Load and hand its Providers to the resolver.
//
// Safe for concurrent use. The listing is built once; a keyring failure met
// while reading a value is recorded so Label and Diagnose can explain it
// instead of every Lookup rediscovering it.
type Source struct {
	store secrets.Store
	// scope is the canonical path of the project directory, and the empty
	// string outside a project or when the path cannot be canonicalized. It is
	// the scope half of every project-tier key.
	scope string

	once   sync.Once
	scoped map[string]string // Airflow env key -> vault key
	global map[string]string
	// down is a whole-source failure: the vault cannot be listed at all.
	down *outage

	mu sync.Mutex
	// values and readErrs cache the outcome of each decrypt, per vault key.
	//
	// A cache rather than a convenience: a declared value was being decrypted
	// twice per start, once by the resolver's Lookup and again by
	// SecretInjection, so every file read and AEAD open happened twice. And the
	// error has to be per key — a source-wide latch mislabeled a value that read
	// back fine as unavailable because some other key had failed earlier.
	values   map[string]string
	readErrs map[string]error
}

// outage is a whole-source failure: a short reason for the list label and a
// longer cause for the missing-value message.
type outage struct {
	short string
	cause string
}

// Load opens the shared vault for one invocation. projectDir is the project's
// directory, or empty outside a project.
//
// It never fails: a machine with no reachable keyring has no vault, which is an
// ordinary state for a CLI (headless Linux, most CI, a locked keychain) and
// must leave the file sources working. The condition is recorded and surfaces
// through Label and Diagnose, so a name that needed the vault says why rather
// than reporting a bare absence.
func Load(projectDir string) *Source {
	s := &Source{}
	dir, err := secrets.DefaultDir()
	if err != nil {
		s.down = &outage{short: "no home directory", cause: "your home directory could not be resolved, so the shared vault has no location"}
		return s
	}
	store, err := secrets.NewKeyringStore(secrets.Config{Service: secrets.DefaultService, Dir: dir})
	if err != nil {
		s.down = &outage{short: "unavailable", cause: fmt.Sprintf("the shared vault could not be opened: %v", err)}
		return s
	}
	s.store = store
	if projectDir != "" {
		// A directory that cannot be canonicalized has no project-tier keys
		// either, since whoever wrote them canonicalized the same way. Not an
		// outage: the global tier is still readable.
		if p, cerr := localrt.CanonicalPath(projectDir); cerr == nil {
			s.scope = p
		}
	}
	return s
}

// Providers is this source's slice of the resolution chain, project tier
// first. The project provider is omitted when there is no project, exactly as
// localenv omits the project file.
//
// Where these sit in the whole chain is localenv's business (it assembles it);
// what matters here is that the two are ordered relative to each other.
func (s *Source) Providers() []envresolve.Provider {
	var ps []envresolve.Provider
	if s.scope != "" {
		ps = append(ps, &provider{src: s, label: SourceProject, global: false})
	}
	return append(ps, &provider{src: s, label: SourceGlobal, global: true})
}

// load builds the env-key -> vault-key index for both tiers, once.
func (s *Source) load() {
	s.once.Do(func() {
		s.scoped, s.global = map[string]string{}, map[string]string{}
		if s.down != nil {
			return
		}
		metas, err := s.store.ListMeta()
		if err != nil {
			s.down = &outage{
				short: "unreadable",
				cause: fmt.Sprintf("the shared vault could not be read: %v", err),
			}
			return
		}
		// Parsed first, then sorted BY NAME. Sorting the raw keys sorts
		// "kind:scope:name", where the kind token dominates — "conn" < "env" <
		// "var" — so the winner of a collision would be decided by which kind
		// the entry was rather than by its name. That is not the rule this
		// package documents, and it is not the rule the desktop's overlay
		// applies, which is the whole point: both tools have to pick the same
		// winner or the same vault means different things in each.
		type entry struct {
			vaultKey string
			envKey   string
			name     string
			global   bool
		}
		entries := make([]entry, 0, len(metas))
		for _, m := range metas {
			kind, scope, name, perr := secrets.ParseKey(m.Key)
			if perr != nil {
				// A key this build does not understand. Written by a newer
				// tool, or not ours at all; either way not resolvable here.
				continue
			}
			envKey, ok := envKeyFor(kind, name)
			if !ok {
				continue
			}
			switch {
			case scope == secrets.GlobalScope:
				entries = append(entries, entry{m.Key, envKey, name, true})
			case s.scope != "" && scope == s.scope:
				entries = append(entries, entry{m.Key, envKey, name, false})
			}
		}
		// Last name wins, so a later assignment overwrites an earlier one. Ties
		// on the name itself fall back to the full key, so the order is total
		// and a run cannot depend on directory order.
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].name != entries[j].name {
				return entries[i].name < entries[j].name
			}
			return entries[i].vaultKey < entries[j].vaultKey
		})
		for _, e := range entries {
			if e.global {
				s.global[e.envKey] = e.vaultKey
			} else {
				s.scoped[e.envKey] = e.vaultKey
			}
		}
	})
}

// envKeyFor is the Airflow env-var name a vault entry resolves under: the
// forward encode, never a reverse derivation. ok is false for a kind this
// source does not serve, or a name that cannot be an env var at all.
func envKeyFor(kind secrets.Kind, name string) (string, bool) {
	switch kind {
	case secrets.KindEnv:
		// Validated like the other two, and for the same reason. secrets.Key
		// only rejects a colon, so a peer tool can legitimately store
		// env:<scope>:A=B — and standalone would then set an OS variable named
		// "A=B", while docker would declare it in the compose file and never
		// resolve it. localenv.EnvKeyFor applies this to the file path already.
		return name, airflowenv.ValidEnvKey(name)
	case secrets.KindVar:
		return airflowenv.EnvKeyForVarKey(name), airflowenv.ValidVarKey(name)
	case secrets.KindConn:
		return airflowenv.EnvKeyForConnID(name), airflowenv.ValidConnID(name)
	default:
		return "", false
	}
}

// Tiers lists what each tier holds, for a listing that shows undeclared vault
// entries the way it shows undeclared file entries. It reads the index only and
// decrypts nothing, so it needs no keyring. The project tier comes first and is
// omitted outside a project; a vault that cannot be listed yields empty tiers.
// Where two names share an env key, the one the chain resolves is listed.
func (s *Source) Tiers() []localenv.VaultTier {
	s.load()
	var out []localenv.VaultTier
	if s.scope != "" {
		out = append(out, localenv.VaultTier{Label: SourceProject, Scope: localenv.ScopeProject, Entries: entries(s.scoped)})
	}
	return append(out, localenv.VaultTier{Label: SourceGlobal, Scope: localenv.ScopeGlobal, Entries: entries(s.global)})
}

// entries turns one tier's index into listing entries, sorted by env key.
func entries(index map[string]string) []localenv.VaultEntry {
	out := make([]localenv.VaultEntry, 0, len(index))
	for envKey, vaultKey := range index {
		kind, _, name, err := secrets.ParseKey(vaultKey)
		if err != nil {
			continue // load indexed only keys that parse
		}
		out = append(out, localenv.VaultEntry{Kind: localKind(kind), Name: name, EnvKey: envKey})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EnvKey < out[j].EnvKey })
	return out
}

// localKind is vaultKind's inverse.
func localKind(kind secrets.Kind) localenv.Kind {
	switch kind {
	case secrets.KindEnv:
		return localenv.KindEnv
	case secrets.KindConn:
		return localenv.KindConn
	case secrets.KindVar:
		return localenv.KindVar
	default:
		return ""
	}
}

// index is the tier's env-key -> vault-key map. Caller has called load.
func (s *Source) index(global bool) map[string]string {
	if global {
		return s.global
	}
	return s.scoped
}

// read decrypts one entry, once, caching the outcome for this invocation.
func (s *Source) read(vaultKey string) (string, bool) {
	s.mu.Lock()
	if v, ok := s.values[vaultKey]; ok {
		s.mu.Unlock()
		return v, true
	}
	if _, failed := s.readErrs[vaultKey]; failed {
		s.mu.Unlock()
		return "", false
	}
	s.mu.Unlock()

	// Outside the lock: this can block on a keyring prompt, and a Source is
	// shared by both tier providers.
	v, err := s.store.Get(vaultKey)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		if s.readErrs == nil {
			s.readErrs = map[string]error{}
		}
		s.readErrs[vaultKey] = err
		return "", false
	}
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[vaultKey] = v
	return v, true
}

// readErrFor is the failure recorded for one key, if any. An absent value is not
// one: the listing and the entry both come from the directory, so absence means
// the entry went away between the two.
func (s *Source) readErrFor(vaultKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.readErrs[vaultKey]
	if errors.Is(err, secrets.ErrNotFound) {
		return nil
	}
	return err
}

// SecretInjection is the map to layer into a run's environment, and it must be
// layered as SECRET env rather than plain: docker mode writes Plan.Env into the
// compose file it leaves on disk, so a decrypted credential there would undo
// the reason the vault exists. Plan.SecretEnv is declared in that file without
// a value and handed to the compose process instead (pkg/localrt), and
// standalone treats it as ordinary environment.
//
// The two tiers inject by different rules, mirroring the files they sit beside:
//
//   - The project tier goes in wholesale. These are this project's own secrets,
//     like its .env.
//   - The global tier contributes only what the schema declares, like
//     ~/.astro/env, so a machine-wide secret never leaks into a project that
//     did not ask for it.
//
// That second rule is a deliberate divergence from Astro Desktop, which reaches
// every project a global value is auto-linked to. The link state lives in the
// desktop's own index and not in the vault, so this tool cannot read it and
// must choose a rule of its own; the conservative one matches the CLI's
// existing treatment of its global file. A project that wants a global secret
// declares it, which is portable and visible in review.
//
// Best effort by design. An unreachable keyring yields an empty map rather than
// an error, because the resolver has already decided what a run cannot start
// without: a declared name with no source blocks it through Missing, and an
// undeclared one was never load-bearing.
func (s *Source) SecretInjection(schema *envschema.Schema) map[string]string {
	s.load()
	out := map[string]string{}
	if s.down != nil {
		return out
	}
	for envKey, vk := range s.scoped {
		if v, ok := s.read(vk); ok {
			out[envKey] = v
		}
	}
	// Global second so a project secret of the same name is not overwritten by
	// it. Both maps are keyed by the same env-var name, so this ordering IS the
	// precedence.
	for _, envKey := range envschema.DeclaredEnvKeys(schema) {
		if _, taken := out[envKey]; taken {
			continue
		}
		if vk, ok := s.global[envKey]; ok {
			if v, ok := s.read(vk); ok {
				out[envKey] = v
			}
		}
	}
	return out
}

// provider is one tier as the resolver sees it.
type provider struct {
	src    *Source
	label  string
	global bool
}

// Lookup reports the decrypted value under an Airflow env-var key, and whether
// this tier holds one.
func (p *provider) Lookup(key string) (string, bool) {
	p.src.load()
	if p.src.down != nil {
		return "", false
	}
	vk, ok := p.src.index(p.global)[key]
	if !ok {
		return "", false
	}
	return p.src.read(vk)
}

// Label names the tier.
//
// No "(unavailable: ...)" variant, unlike the Environment Manager provider's.
// That provider is special-cased by the resolver, which reports its label even
// for a miss; these sit in the ordinary chain, where a provider that does not
// hold a value never has Label called at all. A variant that cannot be reached
// is worse than none — it read as covered while the sticky error behind it
// mislabeled values that had decrypted perfectly well. The cause reaches the
// user through Diagnose, which the resolver does consult.
func (p *provider) Label() string { return p.label }

// Diagnose explains why key did not resolve from this tier, for the
// missing-value message. Empty when there is nothing worth saying — a name this
// tier simply does not hold is the ordinary case and gets no note.
func (p *provider) Diagnose(key string) string {
	p.src.load()
	vaultKey, indexed := p.src.index(p.global)[key]
	if p.src.down != nil {
		return p.src.down.cause
	}
	if !indexed {
		return ""
	}
	if err := p.src.readErrFor(vaultKey); err != nil {
		// Same three conditions refusal() separates, and for the same reason:
		// "unlock your keychain" is useless advice for a key that is gone.
		switch {
		case errors.Is(err, secrets.ErrVaultOrphaned):
			return "the shared vault holds it but the master key that decrypts it is gone, so it cannot be " +
				"recovered — a keychain reset or a new login keychain does this"
		case errors.Is(err, secrets.ErrMasterKeyUnusable):
			return "the shared vault holds it but this machine's stored master key is not usable, so it cannot " +
				"be decrypted"
		case errors.Is(err, secrets.ErrKeyringUnavailable):
			return "the shared vault holds it but this machine's keyring is unreachable, so it cannot be decrypted — " +
				"unlock your keychain, or supply the value in the environment instead"
		}
		return fmt.Sprintf("the shared vault holds it but it could not be read: %v", err)
	}
	return ""
}
