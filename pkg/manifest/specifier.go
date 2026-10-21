package manifest

import (
	"regexp"
	"strconv"
	"strings"
)

// RequirementAdmits reports whether a PEP 508 requirement's version specifier
// admits version, a release a constraint pins it to: "pandas" and
// "pandas>=2.3" admit 2.3.3, "pandas>=3" does not.
//
// known is false when the requirement says more than this reads with
// certainty, and then admits says nothing: an environment marker, which
// limits it to some installs; a direct reference ("name @ url"); "===";
// an epoch; or a version, on either side, PEP 440 does not spell. A caller
// that cannot tell treats the version as excluded, which is the safe answer
// for a pin: without it uv resolves freely.
//
// The ordering is PEP 440's: a dev release sorts before a pre-release, which
// sorts before the final release, which sorts before a post-release. A local
// version (+...) is compared without its local part, as a specifier with
// none does.
func RequirementAdmits(req, version string) (admits, known bool) {
	spec, ok := requirementSpecifier(req)
	if !ok {
		return false, false
	}
	v, ok := parsePEP440(version)
	if !ok {
		return false, false
	}
	if spec == "" {
		return true, true
	}
	for _, clause := range strings.Split(spec, ",") {
		admits, known := clauseAdmits(strings.TrimSpace(clause), v)
		if !known {
			return false, false
		}
		if !admits {
			return false, true
		}
	}
	return true, true
}

// requirementSpecifier is the version specifier of a requirement, "" for one
// with none. ok is false for a requirement this does not read with
// certainty: one with a marker, or a direct reference.
func requirementSpecifier(req string) (string, bool) {
	s := strings.TrimSpace(req)
	if strings.ContainsAny(s, ";@") {
		return "", false
	}
	end := strings.IndexAny(s, " \t[<>=!~(")
	if end < 0 {
		return "", true
	}
	s = strings.TrimSpace(s[end:])
	if strings.HasPrefix(s, "[") {
		bracket := strings.Index(s, "]")
		if bracket < 0 {
			return "", false
		}
		s = strings.TrimSpace(s[bracket+1:])
	}
	if strings.HasPrefix(s, "(") {
		if !strings.HasSuffix(s, ")") {
			return "", false
		}
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	return s, true
}

// specifierOperators is longest first, so "<=" is not read as "<".
var specifierOperators = []string{"===", "~=", "==", "!=", "<=", ">=", "<", ">"}

func clauseAdmits(clause string, v pep440) (admits, known bool) {
	op := ""
	for _, o := range specifierOperators {
		if strings.HasPrefix(clause, o) {
			op = o
			break
		}
	}
	if op == "" || op == "===" {
		return false, false
	}
	text := strings.TrimSpace(strings.TrimPrefix(clause, op))
	if prefix, wild := strings.CutSuffix(text, ".*"); wild {
		return wildcardAdmits(op, prefix, v)
	}
	s, ok := parsePEP440(text)
	if !ok {
		return false, false
	}
	return compareAdmits(op, v, s)
}

// wildcardAdmits is clauseAdmits for "==X.*" and "!=X.*", the two operators
// a wildcard is allowed with: a match on the release's leading segments.
func wildcardAdmits(op, prefix string, v pep440) (admits, known bool) {
	if op != "==" && op != "!=" {
		return false, false
	}
	p, ok := parsePEP440(prefix)
	if !ok || p.hasPre || p.hasPost || p.hasDev {
		return false, false
	}
	match := hasReleasePrefix(v.release, p.release)
	return match == (op == "=="), true
}

// compareAdmits is clauseAdmits for a clause naming one version, s.
func compareAdmits(op string, v, s pep440) (admits, known bool) {
	switch op {
	case "==":
		return comparePEP440(v, s) == 0, true
	case "!=":
		return comparePEP440(v, s) != 0, true
	case ">=":
		return comparePEP440(v, s) >= 0, true
	case "<=":
		return comparePEP440(v, s) <= 0, true
	case ">":
		// A post-release of the version given is not "greater" (PEP 440).
		if !s.hasPost && v.hasPost && sameRelease(v.release, s.release) {
			return false, true
		}
		return comparePEP440(v, s) > 0, true
	case "<":
		// Nor is a pre-release of it "less", unless the version given is one.
		if !s.hasPre && !s.hasDev && (v.hasPre || v.hasDev) && sameRelease(v.release, s.release) {
			return false, true
		}
		return comparePEP440(v, s) < 0, true
	case "~=":
		if len(s.release) < 2 {
			return false, false
		}
		return comparePEP440(v, s) >= 0 && hasReleasePrefix(v.release, s.release[:len(s.release)-1]), true
	}
	return false, false
}

// pep440 is a version as PEP 440 orders it: the release segments, and the
// pre-release, post-release and dev-release parts.
type pep440 struct {
	release                 []int
	hasPre, hasPost, hasDev bool
	preKind                 int // 0 a, 1 b, 2 rc
	pre, post, dev          int
}

var pep440Re = regexp.MustCompile(`^v?(\d+(?:\.\d+)*)` +
	`(?:[-_.]?(a|alpha|b|beta|c|rc|pre|preview)[-_.]?(\d*))?` +
	`(?:-(\d+)|[-_.]?(post|rev|r)[-_.]?(\d*))?` +
	`(?:[-_.]?(dev)[-_.]?(\d*))?` +
	`(?:\+[a-z0-9]+(?:[-_.][a-z0-9]+)*)?$`)

// parsePEP440 reads a version PEP 440 spells, its local part dropped. An
// epoch ("1!2.0") is not read.
func parsePEP440(text string) (pep440, bool) {
	m := pep440Re.FindStringSubmatch(strings.ToLower(strings.TrimSpace(text)))
	if m == nil {
		return pep440{}, false
	}
	var v pep440
	for _, part := range strings.Split(m[1], ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return pep440{}, false
		}
		v.release = append(v.release, n)
	}
	if m[2] != "" {
		v.hasPre = true
		switch m[2] {
		case "a", "alpha":
			v.preKind = 0
		case "b", "beta":
			v.preKind = 1
		default:
			v.preKind = 2
		}
		v.pre = atoiOr0(m[3])
	}
	switch {
	case m[4] != "":
		v.hasPost, v.post = true, atoiOr0(m[4])
	case m[5] != "":
		v.hasPost, v.post = true, atoiOr0(m[6])
	}
	if m[7] != "" {
		v.hasDev, v.dev = true, atoiOr0(m[8])
	}
	return v, true
}

func atoiOr0(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// comparePEP440 orders two versions as PEP 440 does.
func comparePEP440(a, b pep440) int {
	if c := compareRelease(a.release, b.release); c != 0 {
		return c
	}
	for _, pair := range [][2][]int{
		{preKey(a), preKey(b)},
		{postKey(a), postKey(b)},
		{devKey(a), devKey(b)},
	} {
		if c := compareRelease(pair[0], pair[1]); c != 0 {
			return c
		}
	}
	return 0
}

// preKey sorts a dev release of a final release before its pre-releases,
// the pre-releases by kind and number, and the final release after them.
func preKey(v pep440) []int {
	switch {
	case v.hasPre:
		return []int{1, v.preKind, v.pre}
	case v.hasDev && !v.hasPost:
		return []int{0}
	default:
		return []int{2}
	}
}

// postKey sorts no post-release before any.
func postKey(v pep440) []int {
	if !v.hasPost {
		return []int{0}
	}
	return []int{1, v.post}
}

// devKey sorts a dev release before the release it leads to.
func devKey(v pep440) []int {
	if !v.hasDev {
		return []int{1}
	}
	return []int{0, v.dev}
}

// compareRelease orders two lists of numbers, the shorter padded with zeros.
func compareRelease(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func sameRelease(a, b []int) bool { return compareRelease(a, b) == 0 }

// hasReleasePrefix reports whether release starts with prefix, padding
// release with zeros: 3.0 has the prefix 3.0.0 and 3.
func hasReleasePrefix(release, prefix []int) bool {
	for i, p := range prefix {
		r := 0
		if i < len(release) {
			r = release[i]
		}
		if r != p {
			return false
		}
	}
	return true
}
