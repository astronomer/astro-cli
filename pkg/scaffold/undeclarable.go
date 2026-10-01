package scaffold

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// ErrManifestUnloadable is CheckUndeclarable's refusal for a pyproject.toml
// that does not load, or whose [tool.astro.env] does not parse. The error
// returned reads as the load or parse error itself; this sentinel is what a
// caller matches.
var ErrManifestUnloadable = errors.New("the manifest does not load")

// ErrNotDeclared is CheckUndeclarable's refusal for a name the manifest does
// not declare in the section.
var ErrNotDeclared = errors.New("not declared")

// unloadableError carries the load or parse error as its text and matches
// both it and ErrManifestUnloadable.
type unloadableError struct{ err error }

func (e *unloadableError) Error() string   { return e.err.Error() }
func (e *unloadableError) Unwrap() []error { return []error{ErrManifestUnloadable, e.err} }

// CheckUndeclarable reports whether RemoveEnvDeclaration of name from section
// in the pyproject.toml in dir would remove something, without writing. A
// caller that deletes a value and its declaration together calls it first, so
// a declaration that cannot be removed refuses the whole operation before the
// value goes.
//
// It refuses with an error matching ErrManifestUnloadable when the manifest
// does not load or its [tool.astro.env] does not parse, and with one matching
// ErrNotDeclared when the section does not declare name, looked up the way
// EnvDeclarationKey looks it up.
func CheckUndeclarable(dir string, section envschema.Section, name string) error {
	path := filepath.Join(dir, manifest.Marker)
	m, err := manifest.Load(path)
	if err != nil {
		return &unloadableError{err}
	}
	if _, err := envschema.ParseSchema(m.Astro.Env); err != nil {
		return &unloadableError{err}
	}
	_, declared, err := EnvDeclarationKey(dir, section, name)
	if err != nil {
		return err
	}
	if !declared {
		return fmt.Errorf("%s is %w in %s", name, ErrNotDeclared, path)
	}
	return nil
}
