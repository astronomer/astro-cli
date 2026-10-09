package shipcontext

import (
	"crypto/rand"
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
	"*.p8",
	"*.pfx",
	"*.jks",
	"*.keystore",
	"*.ppk",
	"id_rsa*",
	"id_ed25519*",
	"id_ecdsa*",
	"id_dsa*",
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
	"*.tfvars",
	"*.tfstate",
	".env*",
	"*.env",
	"*.env.*",
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

// listShown caps how many files a message names.
const listShown = 10

// CappedList joins paths for a message, naming at most ten and counting the
// rest.
func CappedList(paths []string) string {
	if len(paths) <= listShown {
		return strings.Join(paths, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(paths[:listShown], ", "), len(paths)-listShown)
}

// SecretsError refuses a build that would bake files that look like
// credentials, and that git ignores or could not vouch for, into an image
// bound for a registry; nil when there are none.
func (s *Survey) SecretsError() error {
	if len(s.Secrets) == 0 {
		return nil
	}
	why := "are ignored by git"
	if len(s.GitSkipped) > 0 {
		why = "are ignored by git, or sit where git could not say whether they are tracked"
	}
	return fmt.Errorf("refusing to build: %s look like credentials, %s, and would be baked into an image pushed to a registry. Add them to .dockerignore, or remove them from the project, and try again",
		CappedList(s.Secrets), why)
}

// Warnings are what a build of the survey's files goes ahead despite: files
// git ignores that will ship, and repositories git could not answer for.
func (s *Survey) Warnings() []string {
	var out []string
	if len(s.Gitignored) > 0 {
		out = append(out, fmt.Sprintf("the image will carry %d file(s) that .gitignore ignores and .dockerignore does not: %s. Add them to .dockerignore to keep them out of the image",
			len(s.Gitignored), CappedList(s.Gitignored)))
	}
	for _, skipped := range s.GitSkipped {
		out = append(out, fmt.Sprintf("the check for gitignored credentials was skipped for %s; every file that looks like a credential is refused there instead", skipped))
	}
	return out
}

// Nonce is a short random hex string, for a build's tag to be its own.
func Nonce() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%x", b)
}
