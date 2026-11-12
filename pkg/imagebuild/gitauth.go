package imagebuild

import (
	"fmt"
	"regexp"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// gitAuthFailure is git's own line when it cannot log in to fetch a git
// dependency over https: no credentials at all, or ones the host refused.
var gitAuthFailure = regexp.MustCompile(`(?:could not read Username for|Authentication failed for) 'https?://(?:[^@/']*@)?([^/':]+)`)

// gitAuthHost watches build output for gitAuthFailure and keeps the first
// host it names.
type gitAuthHost struct{ host string }

func (g *gitAuthHost) scan(line string) {
	if g.host != "" {
		return
	}
	if m := gitAuthFailure.FindStringSubmatch(line); m != nil {
		g.host = m[1]
	}
}

// explain replaces a failed build's error when git could not log in to
// g.host: the fix is a netrc build secret, or, when one was passed, its
// credentials for that host. Any other failure comes back as it is.
func (g *gitAuthHost) explain(err error, secrets []string) error {
	if g.host == "" {
		return err
	}
	for _, spec := range secrets {
		if s, perr := manifest.ParseBuildSecret(spec); perr == nil && s.ID == manifest.RuntimeSecretID {
			return fmt.Errorf("the image's install couldn't log in to %s to fetch a git dependency: the %s build secret's credentials for %s were refused: %w",
				g.host, manifest.RuntimeSecretID, g.host, err)
		}
	}
	return fmt.Errorf("the image's install couldn't log in to %s to fetch a git dependency. Pass a netrc build secret:\n"+
		"  --build-secret id=netrc,env=NETRC   (or build-secrets = ['id=netrc,env=NETRC'] in pyproject.toml): %w", g.host, err)
}
