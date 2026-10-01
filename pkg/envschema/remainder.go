package envschema

// Remainder is what is left of a name once its value is deleted from a store.
// Deleting a value never touches the declaration, so a name the manifest
// declares is still a requirement afterwards, and a caller reporting the delete
// says which of these it now is.
type Remainder string

const (
	// RemainderUndeclared: no declaration, so nothing is left to report.
	RemainderUndeclared Remainder = "undeclared"
	// RemainderSupplied: another source (a tier, the workspace, or the
	// declaration's default) still supplies the name.
	RemainderSupplied Remainder = "supplied"
	// RemainderAbsent: declared optional and supplied by nothing, so a list
	// shows it as absent and a start goes on without it.
	RemainderAbsent Remainder = "absent"
	// RemainderRequired: declared required and supplied by nothing, so the
	// next start refuses until a value is set.
	RemainderRequired Remainder = "required"
)

// RemainderAfterDelete classifies a name after its value was deleted. spec is
// its declaration, nil when the manifest does not declare it. supplied is
// whether the caller's resolver still finds a value for it anywhere: another
// tier, the workspace, or the declaration's default.
func RemainderAfterDelete(spec *ValueSpec, supplied bool) Remainder {
	switch {
	case spec == nil:
		return RemainderUndeclared
	case supplied:
		return RemainderSupplied
	case spec.Optional:
		return RemainderAbsent
	}
	return RemainderRequired
}
