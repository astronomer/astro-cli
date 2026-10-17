package cmd

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
)

func TestOttoMembership(t *testing.T) {
	orgResp := func(status int) *astrov1.GetOrganizationResponse {
		return &astrov1.GetOrganizationResponse{HTTPResponse: &http.Response{StatusCode: status}}
	}
	selfUser := &astrov1.GetSelfUserResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &astrov1.SelfUser{Username: "b@example.com"},
	}
	// An API token is no user: the self-user lookup refuses it.
	noUser := &astrov1.GetSelfUserResponse{HTTPResponse: &http.Response{StatusCode: http.StatusUnauthorized}}

	cases := []struct {
		name      string
		org       *astrov1.GetOrganizationResponse
		orgErr    error
		self      *astrov1.GetSelfUserResponse
		selfErr   error
		user      string
		notMember bool
	}{
		{name: "a member", org: orgResp(http.StatusOK)},
		{name: "a user refused the organization", org: orgResp(http.StatusForbidden), self: selfUser, user: "b@example.com", notMember: true},
		{name: "a user the organization is hidden from", org: orgResp(http.StatusNotFound), self: selfUser, user: "b@example.com", notMember: true},
		{name: "an API token refused the organization", org: orgResp(http.StatusForbidden), self: noUser},
		{name: "a refusal whose user cannot be looked up", org: orgResp(http.StatusForbidden), selfErr: errors.New("network error")},
		{name: "a request error", orgErr: errors.New("network error")},
		{name: "a server error", org: orgResp(http.StatusInternalServerError)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := new(astrov1_mocks.ClientWithResponsesInterface)
			client.On("GetOrganizationWithResponse", mock.Anything, "org-a", mock.Anything).Return(tc.org, tc.orgErr).Once()
			if tc.self != nil || tc.selfErr != nil {
				client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(tc.self, tc.selfErr).Once()
			}

			user, notMember := ottoMembership(client)(context.Background(), "org-a")

			assert.Equal(t, tc.user, user)
			assert.Equal(t, tc.notMember, notMember)
			client.AssertExpectations(t)
		})
	}
}
