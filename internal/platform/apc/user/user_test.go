package user

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	houstonMocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	"github.com/astronomer/astro-cli/pkg/input"
)

// Houston's own words when public sign-ups are off, the default.
var errSignupsDisabled = errors.New("Public sign ups are disabled, a valid inviteToken is required to login to the platform. Public sign ups can be enabled via configuration change.")

type Suite struct {
	suite.Suite
}

func TestUser(t *testing.T) {
	suite.Run(t, new(Suite))
}

// createUser answers with the user it made, loaded by id, and a token that
// is null unless the user is active.
func created(status string) *houston.AuthUser {
	return &houston.AuthUser{User: houston.User{ID: "u-1", Username: "test@test.com", Status: status}}
}

func (s *Suite) TestCreateSuccess() {
	houstonMock := new(houstonMocks.ClientInterface)
	houstonMock.On("CreateUser", houston.CreateUserRequest{Email: "test@test.com", Password: "test"}).Return(created("active"), nil)

	got, err := Create("test@test.com", "test", houstonMock)
	s.NoError(err)
	s.Equal(Created{ID: "u-1", Username: "test@test.com", Status: "active", Email: "test@test.com"}, got)
	houstonMock.AssertExpectations(s.T())
}

// With no password given, the prompt asks for a hidden answer, which is never
// read from a stdin that is not a terminal: input_required naming --password,
// and no user is created.
func (s *Suite) TestCreatePrompted() {
	defer input.SetTerminals(false, true)()
	defer input.SetGuard(func() string { return input.Refusal(false, nil) })()
	houstonMock := new(houstonMocks.ClientInterface)

	_, err := Create("someone@example.com", "", houstonMock)
	s.True(input.IsRequired(err), "%v", err)
	s.Contains(err.Error(), "--password")
	houstonMock.AssertNotCalled(s.T(), "CreateUser", mock.Anything)
}

// The failure is Houston's, as it gave it. It used to be "user creation is
// disabled" whatever Houston said: an email in use, a password it refused.
func (s *Suite) TestCreateFailure() {
	houstonMock := new(houstonMocks.ClientInterface)
	houstonMock.On("CreateUser", mock.Anything).Return(nil, errSignupsDisabled)

	_, err := Create("test@test.com", "test", houstonMock)
	s.ErrorIs(err, errSignupsDisabled)
}

func (s *Suite) TestCreateNoAnswer() {
	houstonMock := new(houstonMocks.ClientInterface)
	houstonMock.On("CreateUser", mock.Anything).Return(nil, nil)

	_, err := Create("test@test.com", "test", houstonMock)
	s.ErrorIs(err, errNoUserCreated)
}
