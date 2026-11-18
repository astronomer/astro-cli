package astro

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/project"
)

// environmentDomain returns the host an ASTRO_API_TOKEN run is on, by the one
// rule of a token run (astrosession.TokenHost): the project manifest's
// `domain`, else ASTRO_DOMAIN, else astronomer.io. The saved context's host is
// not consulted, so a machine whose current context is dev still sends a token
// to the host the run names.
//
// Setup runs for every command, so the manifest's domain is read leniently: a
// pyproject.toml whose TOML parses names its `[tool.astro] domain` even when
// another key fails validation, so a command that is no project's (astro
// organization list) still works in a directory with a manifest to fix, and
// the commands that load the manifest report its problems themselves.
//
// A host is never guessed. A pyproject.toml that was found but whose TOML does
// not parse might name a domain, and the manifest's domain outranks
// ASTRO_DOMAIN, so the run stops before anything is sent. Only a directory
// with no pyproject.toml at all, or one with no [tool.astro] domain, falls
// through to ASTRO_DOMAIN and the default.
func environmentDomain() (string, error) {
	domain, err := manifestDomain(config.WorkingPath)
	if err != nil {
		return "", err
	}
	return astrosession.TokenHost("", domain), nil
}

// manifestDomain is the `[tool.astro] domain` of the pyproject.toml of the
// project around dir, "" when there is no project or it names none.
func manifestDomain(dir string) (string, error) {
	proj, err := project.Discover(dir)
	var notFound *project.NotFoundError
	if errors.As(err, &notFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("cannot tell which Astro host ASTRO_API_TOKEN is for: looking for the project's pyproject.toml: %w", err)
	}
	path := filepath.Join(proj.Dir, project.Marker)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot tell which Astro host ASTRO_API_TOKEN is for: reading %s: %w", path, err)
	}
	var doc struct {
		Tool struct {
			Astro struct {
				Domain string `toml:"domain"`
			} `toml:"astro"`
		} `toml:"tool"`
	}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("cannot tell which Astro host ASTRO_API_TOKEN is for: %s is not valid TOML or its [tool.astro] domain is not a string (%w); fix it, or run outside the project", path, err)
	}
	return doc.Tool.Astro.Domain, nil
}
