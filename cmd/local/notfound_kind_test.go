package local

import (
	"errors"
	"fmt"
	"testing"
)

// A refusal that names something missing or already there is classified,
// and its text is untouched.
func TestNotFoundAndExistsAreClassified(t *testing.T) {
	nf := notFoundf("variable %q is not set in %s", "NOPE", "project")
	if got := nf.Error(); got != `variable "NOPE" is not set in project` {
		t.Errorf("message = %q", got)
	}
	if !IsNotFound(fmt.Errorf("wrapped: %w", nf)) || IsConflict(nf) {
		t.Errorf("not-set refusal should be not found only")
	}
	ex := existsf("a link named %s already exists", "prod")
	if !IsConflict(fmt.Errorf("wrapped: %w", ex)) || IsNotFound(ex) {
		t.Errorf("already-exists refusal should be a conflict only")
	}
	if !IsNotFound(fmt.Errorf("variable is %w in project", errValueNotSet)) {
		t.Errorf("delete's not-set error should be not found")
	}
	if IsNotFound(errors.New("boom")) || IsConflict(errors.New("boom")) {
		t.Errorf("an unrelated error must not be classified")
	}
}
