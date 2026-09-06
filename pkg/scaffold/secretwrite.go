package scaffold

import (
	"fmt"

	"github.com/astronomer/astro-cli/pkg/secrets"
)

// SecretWrite is one value the conversion stores in the shared vault at
// ~/.astro/secrets — the same store `astro local env set --secret` writes and
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

// applySecrets stores every carried value.
//
// Runs before any file is written. A conversion that stores nothing and writes
// no manifest is a conversion that did not happen; one that writes a manifest
// full of required connections and then fails to store their values has left a
// project that will not start, with nothing on screen saying why.
//
// A changeset with values and no writer is refused rather than skipped. Silently
// dropping them would produce exactly the state above — a converted project
// declaring connections nothing supplies — from a run that reported success.
func (cs *Changeset) applySecrets() error {
	if len(cs.Secrets) == 0 {
		return nil
	}
	if cs.secrets == nil {
		return fmt.Errorf("%w: %s carries %s to store", ErrNoSecretWriter,
			SettingsRelPath, plural(len(cs.Secrets), "value", "values"))
	}
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
		// A value already in the vault is not overwritten, and this is the one
		// place the two credentials for one connection are ever compared.
		//
		// What is carried here is whatever was committed to a v1 file, which
		// may be months stale or a placeholder. What is already in the vault
		// was put there deliberately, by this user, through `astro local env
		// set --secret` or the app. Writing the committed one over it destroys
		// the good credential unrecoverably and leaves the project running
		// against exactly the value this transform exists to get out of version
		// control.
		held, err := cs.secrets.HasSecret(w.Kind, w.Name)
		if err != nil {
			return fmt.Errorf("check %s: %w", w.Label, err)
		}
		if held {
			cs.Advisories = append(cs.Advisories, w.Name+
				": kept the value already in your vault. "+SettingsRelPath+
				" holds one too, which was not carried over it")
			continue
		}
		if err := cs.secrets.SetSecret(w.Kind, w.Name, w.value); err != nil {
			return fmt.Errorf("store %s: %w", w.Label, err)
		}
	}
	return nil
}
