// Package apitokentest holds what the token packages' tests (deployment,
// workspace-token, organization) share about the token API: matchers for the
// requests a token command sends, so each package checks them the same way.
// Only tests import it.
package apitokentest

import (
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

// GotAs is resp, a GetApiToken answer, as the answer for the token id: the
// same token, carrying id. A test that expects the command to look up the
// token a pick or a name chose answers with it, so the calls the command
// makes next can be held to that id too.
func GotAs(resp *astrov1.GetApiTokenResponse, id string) *astrov1.GetApiTokenResponse {
	got := *resp
	tok := *resp.JSON200
	tok.Id = id
	got.JSON200 = &tok
	return &got
}

// RenamesTo matches the body of a token update that sends name and
// description, for an expectation on UpdateApiTokenWithResponse.
func RenamesTo(name, description string) any {
	return mock.MatchedBy(func(b astrov1.UpdateApiTokenJSONRequestBody) bool {
		return b.Name == name && b.Description != nil && *b.Description == description
	})
}
