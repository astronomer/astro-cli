package scaffold

import (
	"fmt"
	"slices"

	"github.com/astronomer/astro-cli/pkg/secrets"
)

// SecretWrite is one value the conversion stores in the shared vault at
// ~/.astro/secrets — the same store `astro local env <noun> set --secret` writes and
// `astro local start` resolves.
//
// It is not a Change, and that is the point. A Change is bytes for a path
// inside the project, and every consumer of a changeset treats it that way: the
// desktop renders one as a diff, `astro init --json` prints one. A vault write
// is neither a path in the project nor bytes anyone may look at, and modeling
// it as a Change would put a credential into a preview payload.
//
// So the value is unexported and carries no JSON tag: what serializes is the
// kind, the name, and a label to show a person. A caller can say "this will
// store the connection warehouse" and cannot say what it will store.
type SecretWrite struct {
	// Kind is the vault kind, which decides the env var the value resolves to.
	Kind secrets.Kind `json:"kind"`
	// Name is the connection id or variable key.
	Name string `json:"name"`
	// Label is what to show a person, already phrased for a list.
	Label string `json:"label"`

	// value is the plaintext. Unexported so no encoder can reach it.
	value string
}

// SecretWriter stores a converted value in the vault, at a scope IT decides.
//
// The scope is deliberately not this package's business. A vault key is
// (kind, scope, name), and the scope is the canonical path of the project
// directory — which every tool already derives for its own vault work, through
// localrt.CanonicalPath: the CLI in vaultenv.NewWriter, the app in its own vault
// package. Canonicalizing here would make this a third derivation of a rule two
// tools already agree on, and would cost a scaffolding module a dependency on
// the whole local runtime to do it.
//
// So the caller passes a writer that is already scoped to the project, and this
// package says only what to store. Narrow because the conversion only ever adds.
type SecretWriter interface {
	// SetSecret stores value under kind and name. The scope is the writer's.
	SetSecret(kind secrets.Kind, name, value string) error
	// HasSecret reports whether this scope already holds a value for kind and
	// name. It exists so the conversion can decline to overwrite one, which is
	// a policy this package owns rather than each writer — see applySecrets.
	HasSecret(kind secrets.Kind, name string) (bool, error)
}

// SecretValueChecker is what a SecretWriter may also implement so the
// conversion can tell a held value equal to the one it carries from a
// different one. Optional: a writer with only HasSecret has every held value
// treated as different, which is the safe reading.
//
// An equal value is not a conflict. Storing it changes nothing a project
// resolves, so it is carried like any other, SetSecret included (a writer that
// holds it somewhere other than its vault, like Astro Desktop's .env, can move
// it), and airflow_settings.yaml can be retired. A different value is kept
// and the file stays, as HasSecret alone decides.
//
// Plan calls it, not only Apply, so an implementation may decrypt at plan
// time, and the CLI's does: a preview of a conversion then reads the vault's
// value of each name the vault holds. A writer that must not touch the
// keyring during a preview (Astro Desktop's) should compare only what it can
// read as plaintext, such as a .env, and for a name held in the vault return
// Held with Compared false. That is the "not compared" answer, and it keeps
// the conservative behavior a writer with only HasSecret gets: the held value
// is kept, the file stays, and the advisory does not claim the two differ.
type SecretValueChecker interface {
	// HasSecretValue reports whether this scope holds a value for kind and
	// name, and, when it compared them, whether it equals value.
	HasSecretValue(kind secrets.Kind, name, value string) (SecretHeld, error)
}

// SecretHeld is what a SecretValueChecker found under one name.
type SecretHeld struct {
	// Held is that the scope holds a value under the name.
	Held bool
	// Compared is that the writer read the held value and compared it. False
	// is "not compared": the held value is treated as different, without
	// saying it is.
	Compared bool
	// Equal is that the held value is the one the conversion carries. Only
	// meaningful when Held and Compared.
	Equal bool
	// Where names the held copy's home for a person, like ".env", when it is
	// not the vault. Empty means the vault.
	Where string
}

// conflicts reports that a held value is one the conversion must not write
// over: any held value not compared and found equal.
func (h SecretHeld) conflicts() bool { return h.Held && !(h.Compared && h.Equal) }

// place is Where as a sentence's subject: "the vault" when it is empty.
func (h SecretHeld) place() string {
	if h.Where == "" {
		return "the vault"
	}
	return h.Where
}

// yours is where the held copy is, as the advisory addressing its owner says it.
func (h SecretHeld) yours() string {
	if h.Where == "" {
		return "your vault"
	}
	return h.Where
}

// other names the file's value beside the held one: "a different one" when the
// writer compared them, and only "one too" when it could not.
func (h SecretHeld) other() string {
	if h.Compared {
		return "a different one"
	}
	return "one too"
}

// checkHeld asks w about one carried value, through HasSecretValue when w
// implements it and HasSecret otherwise.
func checkHeld(w SecretWriter, s *SecretWrite) (SecretHeld, error) {
	if c, ok := w.(SecretValueChecker); ok {
		return c.HasSecretValue(s.Kind, s.Name, s.value)
	}
	has, err := w.HasSecret(s.Kind, s.Name)
	return SecretHeld{Held: has}, err
}

// applySecrets stores every carried value.
//
// Runs before any file is written. A conversion that stores nothing and writes
// no manifest is a conversion that did not happen; one that writes a manifest
// full of required declarations and then fails to store their values has left a
// project that will not start, with nothing on screen saying why.
//
// A SetSecret that fails partway leaves the values stored before it in the
// vault, and a rerun counts those as already held, so it keeps
// airflow_settings.yaml and says the vault already held them. Nothing is lost:
// the plaintext file stays until the user deletes it.
//
// Values with no writer cannot reach here: Plan clears them and says so in a
// note, because a caller without one is declining the carry rather than
// forgetting it. The nil check remains as an assertion of that.
func (cs *Changeset) applySecrets() error {
	if len(cs.Secrets) == 0 || cs.secrets == nil {
		return nil
	}
	held := make([]SecretHeld, len(cs.Secrets))
	var heldNames []string
	for i := range cs.Secrets {
		w := &cs.Secrets[i]
		if w.value == "" {
			// Nothing here is ever planned empty — readAirflowSettings skips an
			// entry with no value rather than carrying a blank one — so an empty
			// value means this changeset is not the one Plan built. A JSON round
			// trip does it: Kind, Name and Label serialize and value cannot, by
			// design, so what comes back is structurally a valid changeset
			// carrying nothing. Storing it would overwrite whatever the user
			// already had under that key with the empty string, and report
			// success. Apply refuses a Change with nil Content for this reason;
			// this is the same refusal.
			return fmt.Errorf("%w: %s carries no value, so this changeset is not the one that was planned",
				ErrChangedOnDisk, w.Label)
		}
		// A different value already held is not overwritten, and this is the
		// one place the two values for one name are ever compared. An equal
		// one is carried as though nothing were held.
		//
		// What is carried here is whatever was written into a v1 file, which
		// may be months stale or a placeholder. What is already in the vault
		// was put there deliberately, by this user, through `astro local env
		// <noun> set --secret` or the app. Writing the committed one over it destroys
		// the good credential unrecoverably and leaves the project running
		// against exactly the value this transform exists to get out of version
		// control.
		h, err := checkHeld(cs.secrets, w)
		if err != nil {
			return fmt.Errorf("check %s: %w", w.Label, err)
		}
		if h.conflicts() {
			held[i] = h
			heldNames = append(heldNames, w.Name)
		}
	}
	// Plan retires the file only when the vault held none of these. A name that
	// arrived since would leave the file as the only copy of its value, and
	// deleting it anyway is not what the preview described.
	if len(heldNames) > 0 && cs.retires(SettingsRelPath) {
		return fmt.Errorf("%w: this project now holds another value for %s, so %s has to stay; convert again",
			ErrChangedOnDisk, joinNames(heldNames), SettingsRelPath)
	}
	for i := range cs.Secrets {
		w := &cs.Secrets[i]
		if held[i].conflicts() {
			cs.Advisories = append(cs.Advisories, w.Name+
				": kept the value already in "+held[i].yours()+". "+SettingsRelPath+
				" holds "+held[i].other()+", which was not carried over it")
			continue
		}
		if err := cs.secrets.SetSecret(w.Kind, w.Name, w.value); err != nil {
			return fmt.Errorf("store %s: %w", w.Label, err)
		}
	}
	return nil
}

// retires reports whether this changeset deletes path.
func (cs *Changeset) retires(path string) bool {
	return slices.ContainsFunc(cs.Changes, func(c Change) bool { return c.Kind == Delete && c.Path == path })
}
