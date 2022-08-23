package user

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	houstonMocks "github.com/astronomer/astro-cli/houston/mocks"
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
		name    string
		args    args
		wantOut string
		wantErr bool
	}{
		{
			name:    "test with email & password provided",
			args:    args{email: "test@test.com", password: "test"},
			wantOut: "Successfully created user test@test.com",
			wantErr: false,
		},
		{
			name:    "test with email & password not provided",
			args:    args{},
			wantOut: "Successfully created user",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			if err := Create(tt.args.email, tt.args.password, houstonMock, out); (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotOut := out.String(); !strings.Contains(gotOut, tt.wantOut) {
				t.Errorf("Create() = %v, want %v", gotOut, tt.wantOut)
			}
		})
	}
	houstonMock.AssertExpectations(t)
}

func TestCreateFailure(t *testing.T) {
	houstonMock := new(houstonMocks.ClientInterface)
	houstonMock.On("CreateUser", mock.Anything).Return(nil, errMockHouston)

	out := &bytes.Buffer{}
	if err := Create("test@test.com", "test", houstonMock, out); !errors.Is(err, errUserCreationDisabled) {
		t.Errorf("Create() error = %v, wantErr %v", err, errUserCreationDisabled)
		return
	}

	houstonMock.AssertExpectations(t)
}

func TestCreatePending(t *testing.T) {
	houstonMock := new(houstonMocks.ClientInterface)
	houstonMock.On("CreateUser", mock.Anything).Return(&houston.AuthUser{User: houston.User{Status: "pending"}}, nil)

	out := &bytes.Buffer{}
	if err := Create("test@test.com", "test", houstonMock, out); err != nil {
		t.Errorf("Create() error = %v, wantErr %v", err, nil)
		return
	}
	if gotOut := out.String(); !strings.Contains(gotOut, "Check your email for a verification.") && !strings.Contains(gotOut, "Successfully created user") {
		t.Errorf("Create() = %v, want %v, %v", gotOut, "Check your email for a verification.", "Successfully created user")
	}

	houstonMock.AssertExpectations(t)
}
