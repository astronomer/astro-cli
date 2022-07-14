package user

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/astronomer/astro-cli/astro-client"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestUserInvite(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	expectedInvite := astro.UserInvite{
		UserID:         "",
		OrganizationID: "",
		OauthInviteID:  "",
		ExpiresAt:      "",
	}
	jsonResponse, err := json.Marshal(expectedInvite)
	assert.NoError(t, err)
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
			Header:     make(http.Header),
		}
	})
	astroAPI := astro.NewAstroClient(client)

	invite, err := CreateInvite("test", "test1", astroAPI)
	assert.NoError(t, err)
	// TODO this feels incorrect.
	// Should we test if client.CreateUserInvite() was called with the correct input ?
	// Both happy path and error cases are tested in astro client already
	// TODO fix the assertion based on discussion
	assert.Equal(t, invite, expectedInvite)
}
