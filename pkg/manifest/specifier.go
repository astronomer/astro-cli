package manifest

import (
	"strings"

	pep440 "github.com/aquasecurity/go-pep440-version"
)

// RequirementAdmits reports whether a PEP 508 requirement's version specifier
// admits version, a release a constraint pins it to: "pandas" and
// "pandas>=2.3" admit 2.3.3, "pandas>=3" does not.
//
// An environment marker is left out: the specifier is judged as if it
// applied everywhere. A caller weighing a pin that carries a marker of its
// own (a runtime pin's python_version) gets the answer for where both apply,
// and where they do not, the requirement only bounds more than it needs to.
//
// known is false when the requirement says more than this reads with
// certainty, and then admits says nothing: a direct reference ("name @ url");
// "===", which PEP 440 defines on the string as written, where the library
// compares its normalized form; or a specifier or version, on either side,
// PEP 440 does not spell. A caller that cannot tell treats the version as
// excluded, which is the safe answer for a pin: without it uv resolves
// freely.
//
// The comparison is PEP 440's, as github.com/aquasecurity/go-pep440-version
// implements it: epochs, pre-, post- and dev releases, wildcards, "~=", and
// the rules that keep "<3" from admitting 3.0rc1 and ">3" from admitting
// 3.post1. A local version (+...) is compared without its local part, as a
// specifier with none does.
func RequirementAdmits(req, version string) (admits, known bool) {
	spec, ok := requirementSpecifier(req)
	if !ok {
		return false, false
	}
	v, err := pep440.Parse(version)
	if err != nil {
		return false, false
	}
	if spec == "" {
		return true, true
	}
	specifiers, err := pep440.NewSpecifiers(spec)
	if err != nil {
		return false, false
	}
	return specifiers.Check(v), true
}

// requirementSpecifier is the version specifier of a requirement, "" for one
// with none, its environment marker left out. ok is false for a requirement
// this does not read with certainty: a direct reference, "===", or the
// library's "||", which PEP 508 does not have.
func requirementSpecifier(req string) (string, bool) {
	s, _, _ := strings.Cut(req, ";")
	s = strings.TrimSpace(s)
	if strings.Contains(s, "@") || strings.Contains(s, "===") || strings.Contains(s, "||") {
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
