package manifest

import (
	"errors"
	"strings"
	"testing"
)

// The tests here are about the code half of a Problem: which rule refused,
// asserted as an identifier rather than as a sentence.
//
// The validation tables hold which dotted key each finding blames. What a key
// cannot say is WHICH rule blamed it: `<link>.deployment` is the key for two
// different refusals and `<link>.environment` for two more, so a caller
// reading the key alone cannot tell "an mwaa link may not carry a deployment
// id" from "an astro link must". The code says which.
//
// The cases themselves live on validationCases and TestAuthValidation's table,
// beside the keys those already assert, rather than in a second table here:
// one fixture list checked from both angles, so a rule added to it answers for
// its code as well as its key.

// problemCodesOf is the code half of problemKeys.
func problemCodesOf(ve *ValidationError) []ProblemCode {
	var codes []ProblemCode
	for _, p := range ve.Problems {
		codes = append(codes, p.Code)
	}
	return codes
}

// Every problem carries both halves: a code to branch on and a sentence to
// read.
//
// Without this each field is optional in practice — a rule added with either
// one left off still compiles and still reports, and the zero value of both
// is the empty string, not an error. The reason half is checked here because
// nothing else checks it any more: the auth table used to assert a substring
// of each message, which pinned the English rather than the rule and is what
// the codes replaced. That trade is only safe if something still notices a
// message going missing altogether.
func TestEveryProblemCarriesACodeAndAReason(t *testing.T) {
	check := func(t *testing.T, content string) {
		t.Helper()
		_, err := Load(write(t, content))
		ve := validationError(t, err)
		for _, p := range ve.Problems {
			if p.Code == "" {
				t.Errorf("a problem on %q carries no code: %q", p.Key, p.Reason)
			}
			if p.Reason == "" {
				t.Errorf("a problem on %q (%s) carries no reason", p.Key, p.Code)
			}
		}
	}
	for _, tc := range validationCases {
		t.Run(tc.name, func(t *testing.T) { check(t, tc.content) })
	}
	for _, tc := range authCases() {
		t.Run(tc.name, func(t *testing.T) { check(t, endpointLink(tc.auth)) })
	}
}

// Every declared code is one some manifest actually produces.
//
// A code nothing raises is either a rule deleted out from under it or one
// whose call site never fires, and both are invisible while the only checks on
// the list are that its members are spelled well and distinct. This is what
// makes the fixture tables answer for the whole set rather than for whichever
// rules somebody remembered: the auth rules had no code assertion at all until
// this test asked for one.
func TestEveryCodeIsReachable(t *testing.T) {
	raised := map[ProblemCode]bool{}
	record := func(content string) {
		_, err := Load(write(t, content))
		var ve *ValidationError
		if !errors.As(err, &ve) {
			return
		}
		for _, p := range ve.Problems {
			raised[p.Code] = true
		}
	}
	for _, tc := range validationCases {
		record(tc.content)
	}
	for _, tc := range authCases() {
		record(endpointLink(tc.auth))
	}
	for _, code := range problemCodes {
		if !raised[code] {
			t.Errorf("no fixture raises %q", code)
		}
	}
}

// A code is a stable identifier, so it is spelled like one: a caller writes
// these into a config file or a jq filter, and one carrying a space or a
// capital is one somebody will quote wrongly.
func TestProblemCodesAreSpelledLikeIdentifiers(t *testing.T) {
	for _, code := range problemCodes {
		got := string(code)
		switch {
		case got == "":
			t.Error("an empty code")
		case got != strings.ToLower(got):
			t.Errorf("%q should be lowercase", got)
		case strings.ContainsAny(got, " \t-."):
			t.Errorf("%q should join its words with underscores", got)
		}
	}
}

// No two codes share a value. A duplicate makes a caller's branch fire on the
// wrong rule, which is the failure a code exists to prevent, reintroduced by
// a copy-paste.
func TestProblemCodesAreDistinct(t *testing.T) {
	seen := map[ProblemCode]bool{}
	for _, code := range problemCodes {
		if seen[code] {
			t.Errorf("%q is declared twice", code)
		}
		seen[code] = true
	}
}
