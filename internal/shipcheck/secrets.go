package shipcheck

import (
	"fmt"
	"path"
	"strings"
)

// secretNames are the base names, as path.Match patterns over the lowercased
// name, of files that commonly hold credentials: keys and certificates,
// cloud and service-account keys, package-registry and kube credentials,
// Terraform variables and state, and .env files.
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

// cappedList joins paths for a message, naming at most ten and counting the
// rest.
func cappedList(paths []string) string {
	if len(paths) <= listShown {
		return strings.Join(paths, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(paths[:listShown], ", "), len(paths)-listShown)
}
