package env

// SetOutcomeKind is what `set --from-file` did with one entry of the file.
type SetOutcomeKind string

const (
	// SetCreated: the key did not exist in the scope and now does.
	SetCreated SetOutcomeKind = "created"
	// SetUpdated: the key existed and took the file's value.
	SetUpdated SetOutcomeKind = "updated"
	// SetSkippedEmpty: the file gave the key an empty value, so it was left
	// alone. An export without --include-secrets writes every secret that
	// way, and setting those would blank the stored values.
	SetSkippedEmpty SetOutcomeKind = "skipped_empty"
	// SetFailed: setting the key failed, which stopped the import there.
	// It is the last outcome; the keys after it were not tried.
	SetFailed SetOutcomeKind = "failed"
)

// SetOutcome is what happened to one key of the file. Object is the object
// as the set left it, absent for a key that was skipped or failed; Error is
// why a failed key failed.
type SetOutcome struct {
	Key    string         `json:"key"`
	Kind   SetOutcomeKind `json:"kind"`
	Object *ObjectInfo    `json:"object,omitempty"`
	Error  string         `json:"error,omitempty"`
}

// SetFromFileResult is what `variable set --from-file` and `airflow-variable
// set --from-file` publish under -o json: one outcome per key of the file
// it reached, in key order, and [] for a file with none. A run that stopped
// on a failure publishes it too, ending in the failed key, and exits 1.
type SetFromFileResult struct {
	Outcomes []SetOutcome `json:"outcomes"`
}
