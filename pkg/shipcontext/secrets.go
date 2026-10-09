package shipcontext

import (
	"fmt"
	"path"
	"strings"
)

// secretNames are the base names, as path.Match patterns over the lowercased
// name, of files that commonly hold credentials: keys and certificates,
// cloud and service-account keys, package-registry and kube credentials,
// and .env files.
var secretNames = []string{
	"*.pem",
	"*.key",
	"*.p12",
	"*.pfx",
	"id_rsa*",
	"*credentials*",
	"*-key.json",
	"*_key.json",
	"service-account*.json",
	"sa-*.json",
	".netrc",
	".npmrc",
	".pypirc",
	"*.kubeconfig",
	"kubeconfig",
	".env*",
}

// secretDirs are directories whose every file is a credential.
var secretDirs = []string{".aws", ".ssh"}

// LooksSecret reports whether the file at p (slash-separated) looks like it
// holds credentials, by its name or a directory it sits in.
func LooksSecret(p string) bool {
	segs := strings.Split(strings.ToLower(p), "/")
	for _, seg := range segs[:len(segs)-1] {
		for _, d := range secretDirs {
			if seg == d {
				return true
			}
		}
	}
	base := segs[len(segs)-1]
	for _, pat := range secretNames {
		// The patterns are this file's own and all well formed, so Match
		// cannot fail on them.
		if ok, err := path.Match(pat, base); err == nil && ok {
			return true
		}
	}
	return false
}

// secretsShown caps how many files SecretsError names.
const secretsShown = 10

// SecretsError refuses a build that would bake gitignored files that look
// like credentials into an image bound for a registry; nil when there are
// none.
func SecretsError(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	shown, more := paths, ""
	if len(shown) > secretsShown {
		shown = shown[:secretsShown]
		more = fmt.Sprintf(" and %d more", len(paths)-secretsShown)
	}
	return fmt.Errorf("refusing to build: %s%s look like credentials, are ignored by git, and would be baked into an image pushed to a registry. Add them to .dockerignore, or remove them from the project, and try again",
		strings.Join(shown, ", "), more)
}
