package deployment

// The result types for a Deployment's environment variables.
//
// They exist because what a modify run did to each input is data, not prose.
// Under the old shape those outcomes were printed — half to the caller's
// writer, half to bare stdout — so a caller capturing the writer saw only
// some of them, and a script that asked for five variables and got one had no way to learn which
// four were skipped. The error said "check the command output above", which
// only answers a human watching a terminal.
//
// The shape follows pkg/checks (Result and Finding): one record per thing that happened, plus
// the counts the caller needs to render a summary and pick an exit code.

// VariableOutcomeKind is what happened to one input.
type VariableOutcomeKind string

const (
	// VariableCreated: the key was not on the Deployment and now is.
	VariableCreated VariableOutcomeKind = "created"
	// VariableUpdated: the key was already there and took a new value.
	VariableUpdated VariableOutcomeKind = "updated"
	// VariableSkippedExists: the key was already there and --update was not
	// given, so it kept its old value.
	VariableSkippedExists VariableOutcomeKind = "skipped_exists"
	// VariableInvalid: the input produced no variable at all.
	VariableInvalid VariableOutcomeKind = "invalid"
)

// VariableOutcome is what happened to one input key. It is the unit the cmd
// layer renders as one line of text. The json tags are for a structured
// caller; these commands have no --output json yet. Fields not relevant to a
// Kind stay zero and omit from JSON.
type VariableOutcome struct {
	Kind VariableOutcomeKind `json:"kind"`
	// Key is the variable this is about. Empty when the input carried no
	// usable key, as in `=value` or a pair with no `=` at all.
	Key string `json:"key,omitempty"`
	// Input is the raw argument an invalid outcome came from, so a caller can
	// point at what the user typed rather than at a key that never existed.
	Input string `json:"input,omitempty"`
	// Reason explains a skip or an invalid input in one sentence.
	Reason string `json:"reason,omitempty"`
}

// VariableInfo is one of a Deployment's environment variables. A secret's
// Value is never carried: the API does not return it, and a masked string is a
// rendering decision that belongs in cmd.
type VariableInfo struct {
	Key      string `json:"key"`
	Value    string `json:"value,omitempty"`
	IsSecret bool   `json:"is_secret"`
}

// DeploymentVariables is a Deployment's environment variables, in the order
// the API returned them.
type DeploymentVariables struct {
	Variables []VariableInfo `json:"variables"`
}

// VariableModifyResult is what a create or update run did: one outcome per
// input, and the Deployment's variables afterwards.
type VariableModifyResult struct {
	Outcomes []VariableOutcome `json:"outcomes"`
	// Variables is the Deployment's list after the update, the same value
	// `variable list` returns.
	Variables []VariableInfo `json:"variables"`
}

// InvalidInputs lists what each invalid input was, for an error that says
// which ones rather than how many.
func (r *VariableModifyResult) InvalidInputs() []string {
	var in []string
	for _, o := range r.Outcomes {
		if o.Kind != VariableInvalid {
			continue
		}
		if o.Input != "" {
			in = append(in, o.Input)
			continue
		}
		in = append(in, o.Key)
	}
	return in
}
