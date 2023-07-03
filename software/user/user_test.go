package user

import (
	"bytes"
	"errors"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	houstonMocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var errMockHouston = errors.New("some houston error")

func TestCreateSuccess(t *testing.T) {
	houstonMock := new(houstonMocks.ClientInterface)
	houstonMock.On("CreateUser", mock.Anything).Return(&houston.AuthUser{}, nil)

	type args struct {
		email    string
		password string
	}
	tests := []struct {
		name         string
		args         args
		wantOut      string
		errAssertion assert.ErrorAssertionFunc
	}{
		{
			name:         "test with email & password provided",
			args:         args{email: "test@test.com", password: "test"},
			wantOut:      "Successfully created user test@test.com",
			errAssertion: assert.NoError,
		},
		{
			name:         "test with email & password not provided",
			args:         args{},
			wantOut:      "Successfully created user",
			errAssertion: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			if tt.errAssertion(t, Create(tt.args.email, tt.args.password, houstonMock, out)) {
				return
			}
			assert.Contains(t, out.String(), tt.wantOut)
		})
	}
	houstonMock.AssertExpectations(t)
}

func TestCreateFailure(t *testing.T) {
	houstonMock := new(houstonMocks.ClientInterface)
	houstonMock.On("CreateUser", mock.Anything).Return(nil, errMockHouston)

	out := &bytes.Buffer{}
	if !assert.ErrorIs(t, Create("test@test.com", "test", houstonMock, out), errUserCreationDisabled) {
		return
	}

	houstonMock.AssertExpectations(t)
}

func TestCreatePending(t *testing.T) {
	houstonMock := new(houstonMocks.ClientInterface)
	houstonMock.On("CreateUser", mock.Anything).Return(&houston.AuthUser{User: houston.User{Status: "pending"}}, nil)

	out := &bytes.Buffer{}
	if !assert.NoError(t, Create("test@test.com", "test", houstonMock, out)) {
		return
	}
	assert.Contains(t, out.String(), "Check your email for a verification.")
	assert.Contains(t, out.String(), "Successfully created user")

	houstonMock.AssertExpectations(t)
}
