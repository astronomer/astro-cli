package user

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	houstonMocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

var errMockHouston = errors.New("some houston error")

type Suite struct {
	suite.Suite
}

func TestSoftwareUserSuite(t *testing.T) {
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
		name    string
		args    args
		wantOut string
	}{
		{
			name:    "test with email & password provided",
			args:    args{email: "test@test.com", password: "test"},
			wantOut: "Successfully created user test@test.com",
		},
		{
			name:    "test with email & password not provided",
			args:    args{},
			wantOut: "Successfully created user",
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			out := &bytes.Buffer{}
			if err := Create(tt.args.email, tt.args.password, houstonMock, out); err != nil {
				s.NoError(err)
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
	s.ErrorIs(Create("test@test.com", "test", houstonMock, out), errUserCreationDisabled)

	houstonMock.AssertExpectations(s.T())
}

func (s *Suite) TestCreatePending() {
	houstonMock := new(houstonMocks.ClientInterface)
	houstonMock.On("CreateUser", mock.Anything).Return(&houston.AuthUser{User: houston.User{Status: "pending"}}, nil)

	out := &bytes.Buffer{}
	if err := Create("test@test.com", "test", houstonMock, out); err != nil {
		s.NoError(err)
		return
	}

	if gotOut := out.String(); !strings.Contains(gotOut, "Check your email for a verification.") && !strings.Contains(gotOut, "Successfully created user") {
		s.Failf("Create()", "got = %v, want %v, %v", gotOut, "Check your email for a verification.", "Successfully created user")
	}

	houstonMock.AssertExpectations(s.T())
}
