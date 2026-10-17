package otto

import (
	"bytes"
	http_context "context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
)

func orgResp(status int) *astrov1.GetOrganizationResponse {
	return &astrov1.GetOrganizationResponse{HTTPResponse: &http.Response{StatusCode: status}}
}

func selfResp(username string) *astrov1.GetSelfUserResponse {
	return &astrov1.GetSelfUserResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.SelfUser{Username: username},
	}
}

func TestEffectiveOrganization(t *testing.T) {
	t.Run("context org wins over env", func(t *testing.T) {
		t.Setenv("ASTRO_ORGANIZATION", "env-org")
		org, fromEnv := effectiveOrganization(&Config{Organization: "ctx-org"})
		assert.Equal(t, "ctx-org", org)
		assert.False(t, fromEnv)
	})

	t.Run("falls back to env when context org empty", func(t *testing.T) {
		t.Setenv("ASTRO_ORGANIZATION", "env-org")
		org, fromEnv := effectiveOrganization(&Config{Organization: ""})
		assert.Equal(t, "env-org", org)
		assert.True(t, fromEnv)
	})

	t.Run("empty when neither set", func(t *testing.T) {
		t.Setenv("ASTRO_ORGANIZATION", "")
		org, _ := effectiveOrganization(&Config{Organization: ""})
		assert.Equal(t, "", org)
	})
}

func TestVerifyOrgMembership(t *testing.T) {
	t.Run("member (200) proceeds without error or output", func(t *testing.T) {
		client := new(astrov1_mocks.ClientWithResponsesInterface)
		client.On("GetOrganizationWithResponse", mock.Anything, "org-a", mock.Anything).
			Return(orgResp(http.StatusOK), nil).Once()

		var out bytes.Buffer
		err := verifyOrgMembership(&Config{Token: "t", Organization: "org-a"}, client, &out)

		assert.NoError(t, err)
		assert.Empty(t, out.String())
		client.AssertExpectations(t)
		// A member must not trigger the self-user lookup.
		client.AssertNotCalled(t, "GetSelfUserWithResponse", mock.Anything, mock.Anything)
	})

	forbiddenStatuses := map[string]int{
		"forbidden": http.StatusForbidden,
		"not found": http.StatusNotFound,
	}
	for name, status := range forbiddenStatuses {
		t.Run(name+" fails closed with actionable, user-named guidance", func(t *testing.T) {
			client := new(astrov1_mocks.ClientWithResponsesInterface)
			client.On("GetOrganizationWithResponse", mock.Anything, "org-a", mock.Anything).
				Return(orgResp(status), nil).Once()
			client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).
				Return(selfResp("b@example.com"), nil).Once()

			var out bytes.Buffer
			err := verifyOrgMembership(&Config{Token: "t", Organization: "org-a"}, client, &out)

			assert.ErrorIs(t, err, ErrNotOrgMember)
			msg := out.String()
			assert.Contains(t, msg, "b@example.com")
			assert.Contains(t, msg, "org-a")
			assert.Contains(t, msg, "astro organization switch")
			assert.NotContains(t, msg, "unset ASTRO_ORGANIZATION")
			client.AssertExpectations(t)
		})
	}

	t.Run("forbidden validates the inherited env org, not the empty context org", func(t *testing.T) {
		t.Setenv("ASTRO_ORGANIZATION", "stale-env-org")
		client := new(astrov1_mocks.ClientWithResponsesInterface)
		client.On("GetOrganizationWithResponse", mock.Anything, "stale-env-org", mock.Anything).
			Return(orgResp(http.StatusForbidden), nil).Once()
		client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).
			Return(selfResp("b@example.com"), nil).Once()

		var out bytes.Buffer
		err := verifyOrgMembership(&Config{Token: "t", Organization: ""}, client, &out)

		assert.ErrorIs(t, err, ErrNotOrgMember)
		msg := out.String()
		assert.Contains(t, msg, "stale-env-org")
		assert.Contains(t, msg, "unset ASTRO_ORGANIZATION")
		assert.NotContains(t, msg, "astro organization switch")
		client.AssertExpectations(t)
	})

	// A 403/404 blocks only a login the self-user lookup can name. An API token
	// reaches an organization by its own grants, not membership.
	t.Run("forbidden for an API token lets the launch proceed", func(t *testing.T) {
		client := new(astrov1_mocks.ClientWithResponsesInterface)
		client.On("GetOrganizationWithResponse", mock.Anything, "org-a", mock.Anything).
			Return(orgResp(http.StatusForbidden), nil).Once()
		client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).
			Return(&astrov1.GetSelfUserResponse{HTTPResponse: &http.Response{StatusCode: http.StatusUnauthorized}}, nil).Once()

		var out bytes.Buffer
		err := verifyOrgMembership(&Config{Token: "t", Organization: "org-a"}, client, &out)

		assert.NoError(t, err)
		assert.Empty(t, out.String())
		client.AssertExpectations(t)
	})

	t.Run("forbidden with failed self-user lookup lets the launch proceed", func(t *testing.T) {
		client := new(astrov1_mocks.ClientWithResponsesInterface)
		client.On("GetOrganizationWithResponse", mock.Anything, "org-a", mock.Anything).
			Return(orgResp(http.StatusForbidden), nil).Once()
		client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).
			Return(nil, errors.New("network error")).Once()

		var out bytes.Buffer
		err := verifyOrgMembership(&Config{Token: "t", Organization: "org-a"}, client, &out)

		assert.NoError(t, err)
		assert.Empty(t, out.String())
		client.AssertExpectations(t)
	})

	t.Run("the API calls run under a deadline", func(t *testing.T) {
		hasDeadline := mock.MatchedBy(func(ctx http_context.Context) bool {
			_, ok := ctx.Deadline()
			return ok
		})
		client := new(astrov1_mocks.ClientWithResponsesInterface)
		client.On("GetOrganizationWithResponse", hasDeadline, "org-a", mock.Anything).
			Return(orgResp(http.StatusOK), nil).Once()

		err := verifyOrgMembership(&Config{Token: "t", Organization: "org-a"}, client, &bytes.Buffer{})

		assert.NoError(t, err)
		client.AssertExpectations(t)
	})

	t.Run("no org to check: skips the API call entirely", func(t *testing.T) {
		t.Setenv("ASTRO_ORGANIZATION", "")
		client := new(astrov1_mocks.ClientWithResponsesInterface)

		var out bytes.Buffer
		err := verifyOrgMembership(&Config{Token: "t", Organization: ""}, client, &out)

		assert.NoError(t, err)
		client.AssertNotCalled(t, "GetOrganizationWithResponse", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("no token: skips the API call entirely", func(t *testing.T) {
		client := new(astrov1_mocks.ClientWithResponsesInterface)

		var out bytes.Buffer
		err := verifyOrgMembership(&Config{Token: "", Organization: "org-a"}, client, &out)

		assert.NoError(t, err)
		client.AssertNotCalled(t, "GetOrganizationWithResponse", mock.Anything, mock.Anything, mock.Anything)
	})

	// Inconclusive outcomes must fail OPEN so the preflight never becomes a new
	// way for `astro otto` to break.
	t.Run("request error lets the launch proceed", func(t *testing.T) {
		client := new(astrov1_mocks.ClientWithResponsesInterface)
		client.On("GetOrganizationWithResponse", mock.Anything, "org-a", mock.Anything).
			Return(nil, errors.New("network error")).Once()

		var out bytes.Buffer
		err := verifyOrgMembership(&Config{Token: "t", Organization: "org-a"}, client, &out)

		assert.NoError(t, err)
		assert.Empty(t, out.String())
		client.AssertExpectations(t)
	})

	t.Run("5xx lets the launch proceed", func(t *testing.T) {
		client := new(astrov1_mocks.ClientWithResponsesInterface)
		client.On("GetOrganizationWithResponse", mock.Anything, "org-a", mock.Anything).
			Return(orgResp(http.StatusInternalServerError), nil).Once()

		var out bytes.Buffer
		err := verifyOrgMembership(&Config{Token: "t", Organization: "org-a"}, client, &out)

		assert.NoError(t, err)
		assert.Empty(t, out.String())
		client.AssertExpectations(t)
	})
}
