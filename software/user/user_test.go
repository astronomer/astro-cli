package user

import (
	"bytes"
	"errors"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	houstonMocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

var errMockHouston = errors.New("some houston error")

type Suite struct {
	suite.Suite
}

func TestUser(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestCreateSuccess() {
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
		s.Run(tt.name, func() {
			out := &bytes.Buffer{}
			if tt.errAssertion(s.T(), Create(tt.args.email, tt.args.password, houstonMock, out)) {
				return
			}
			s.Contains(out.String(), tt.wantOut)
		})
	}
	houstonMock.AssertExpectations(s.T())
}

func (s *Suite) TestCreateFailure() {
	houstonMock := new(houstonMocks.ClientInterface)
	houstonMock.On("CreateUser", mock.Anything).Return(nil, errMockHouston)

	out := &bytes.Buffer{}
	if !s.ErrorIs(Create("test@test.com", "test", houstonMock, out), errUserCreationDisabled) {
		return
	}

	houstonMock.AssertExpectations(s.T())
}

func (s *Suite) TestCreatePending() {
	houstonMock := new(houstonMocks.ClientInterface)
	houstonMock.On("CreateUser", mock.Anything).Return(&houston.AuthUser{User: houston.User{Status: "pending"}}, nil)

	out := &bytes.Buffer{}
	if !s.NoError(Create("test@test.com", "test", houstonMock, out)) {
		return
	}
	s.Contains(out.String(), "Check your email for a verification.")
	s.Contains(out.String(), "Successfully created user")

	houstonMock.AssertExpectations(s.T())
}
