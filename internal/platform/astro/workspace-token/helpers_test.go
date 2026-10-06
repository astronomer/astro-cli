package workspacetoken

import (
	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

// pickIndex is a picker that chooses the token at i, as a person typing its
// number would.
func pickIndex(i int) apitoken.Picker {
	return func(string, []apitoken.Token) (int, error) { return i, nil }
}

// findAndRotate rotates the token id or name means, as the command does once
// a person has confirmed. What a rotate returns is TestRotateTokenResult's.
func findAndRotate(id, name string, client astrov1.APIClient) error {
	ws, org, err := Target("")
	if err != nil {
		return err
	}
	tok, err := FindToken(id, name, ws, org, []TokenType{TokenTypeWORKSPACE}, pickIndex(1), client)
	if err != nil {
		return err
	}
	_, err = RotateToken(tok, ws, client)
	return err
}

// findAndDelete deletes or removes the token id or name means, as the
// command does once a person has confirmed. What a delete returns is
// TestDeleteTokenResult's.
func findAndDelete(id, name string, client astrov1.APIClient) error {
	ws, org, err := Target("")
	if err != nil {
		return err
	}
	tok, err := FindToken(id, name, ws, org, nil, pickIndex(1), client)
	if err != nil {
		return err
	}
	_, err = DeleteToken(tok, ws, client)
	return err
}
