package user

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	"github.com/stretchr/testify/mock"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

var (
	errorWorkspace = errors.New("could not get workspace: test-ws-error")
	errorInvite    = errors.New("test-inv-error")
)

type testWriter struct {
	Error error
}

func (t testWriter) Write(p []byte) (n int, err error) {
	return 0, t.Error
}

func TestCreateInvite(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	inviteUserId := "user_cuid"
	createInviteResponseOK := astrocore.CreateUserInviteResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.Invite{
			InviteId: "",
			UserId:   &inviteUserId,
		},
	}
	errorBody, _ := json.Marshal(astrocore.Error{
		Message: "failed to create invite: test-inv-error",
	})
	createInviteResponseError := astrocore.ListOrganizationsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBody,
		JSON200: nil,
	}
	t.Run("happy path", func(t *testing.T) {
		expectedOutMessage := "invite for test-email@test.com with role ORGANIZATION_MEMBER created\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithBodyWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
	t.Run("error path when CreateUserInviteWithBodyWithResponse returns an error", func(t *testing.T) {
		expectedOutMessage := "failed to create invite: test-inv-error"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithBodyWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseError, nil).Once()
		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.EqualError(t, err, expectedOutMessage)
	})
	t.Run("error path when isValidRole returns an error", func(t *testing.T) {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithBodyWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("test-email@test.com", "test-role", out, mockClient)
		assert.ErrorIs(t, err, ErrInvalidRole)
		assert.Equal(t, expectedOutMessage, out.String())
	})
	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithBodyWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
	t.Run("error path when email is blank returns an error", func(t *testing.T) {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithBodyWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("", "test-role", out, mockClient)
		assert.ErrorIs(t, err, ErrInvalidEmail)
		assert.Equal(t, expectedOutMessage, out.String())
	})
	t.Run("error path when writing output returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateUserInviteWithBodyWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", testWriter{Error: errorInvite}, mockClient)
		assert.EqualError(t, err, "failed to create invite: test-inv-error")
	})
}

func TestIsRoleValid(t *testing.T) {
	var err error
	t.Run("happy path when role is ORGANIZATION_MEMBER", func(t *testing.T) {
		err = IsRoleValid("ORGANIZATION_MEMBER")
		assert.NoError(t, err)
	})
	t.Run("happy path when role is ORGANIZATION_BILLING_ADMIN", func(t *testing.T) {
		err = IsRoleValid("ORGANIZATION_BILLING_ADMIN")
		assert.NoError(t, err)
	})
	t.Run("happy path when role is ORGANIZATION_OWNER", func(t *testing.T) {
		err = IsRoleValid("ORGANIZATION_OWNER")
		assert.NoError(t, err)
	})
	t.Run("error path", func(t *testing.T) {
		err = IsRoleValid("test")
		assert.ErrorIs(t, err, ErrInvalidRole)
	})
}
