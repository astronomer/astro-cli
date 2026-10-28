package shipcheck

import (
	"fmt"
	"path"
	"slices"
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
	".pgpass",
	"token.json",
	"client_secret*.json",
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
	if base == "config.json" && slices.Contains(segs, ".docker") {
		return true // docker registry auth
	}
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

// placesShown caps how many places a summary names.
const placesShown = 3

// placesOf names where the ignored files are rather than the files, so that
// every place it names is safe to add to an ignore file as it stands: each
// file is named by the highest directory above it that holds nothing else the
// image would carry (carried, the whole list), so dags/__pycache__/ and never
// dags/ beside a DAG that ships; a file with no such directory, one at the
// project root or beside a shipped file, is named itself. Most files first,
// at most three, and a count of the rest. A warning about thousands of files
// is read for what to add to an ignore file, and that is a line or two, not a
// list the warning would be lost in.
func placesOf(ignored, carried []string) string {
	isIgnored := make(map[string]bool, len(ignored))
	for _, p := range ignored {
		isIgnored[p] = true
	}
	// shipping are the directories holding a file the image carries for real:
	// naming one of them would leave that file out too.
	shipping := map[string]bool{}
	for _, f := range carried {
		if isIgnored[f] {
			continue
		}
		for i := strings.IndexByte(f, '/'); i >= 0; i = nextSlash(f, i) {
			shipping[f[:i+1]] = true
		}
	}
	counts := map[string]int{}
	for _, p := range ignored {
		place := p
		for i := strings.IndexByte(p, '/'); i >= 0; i = nextSlash(p, i) {
			if !shipping[p[:i+1]] {
				place = p[:i+1]
				break
			}
		}
		counts[place]++
	}
	places := make([]string, 0, len(counts))
	for place := range counts {
		places = append(places, place)
	}
	slices.SortFunc(places, func(a, b string) int {
		if counts[a] != counts[b] {
			return counts[b] - counts[a]
		}
		return strings.Compare(a, b)
	})
	if len(places) <= placesShown {
		return joinAnd(places)
	}
	more := fmt.Sprintf("%d more places", len(places)-placesShown)
	if len(places)-placesShown == 1 {
		more = "1 more place"
	}
	return strings.Join(places[:placesShown], ", ") + " and " + more
}

// nextSlash is the index of the next "/" in p after the one at i, or -1.
func nextSlash(p string, i int) int {
	j := strings.IndexByte(p[i+1:], '/')
	if j < 0 {
		return -1
	}
	return i + 1 + j
}

// joinAnd joins items as a sentence does: "a", "a and b", "a, b and c".
func joinAnd(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// countFiles is n with its noun and thousands separators: "1 file",
// "5,510 files".
func countFiles(n int) string {
	if n == 1 {
		return "1 file"
	}
	digits := fmt.Sprint(n)
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	return b.String() + " files"
}
