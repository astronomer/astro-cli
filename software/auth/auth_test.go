package auth

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/mocks"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/houston"
	houstonMocks "github.com/astronomer/astro-cli/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const (
	mockToken = "token"
)

var (
	errMockRegistry     = errors.New("some error on registry side")
	errMockHouston      = errors.New("some error on houston side")
	errInvalidWorkspace = errors.New("last used workspace id is not valid")
)

type Suite struct {
	suite.Suite

	origSwitchToLastUsedWorkspace func(client houston.ClientInterface, c *config.Context) bool
}

func TestAuth(t *testing.T) {
	suite.Run(t, new(Suite))
}

var (
	_ suite.SetupAllSuite     = (*Suite)(nil)
	_ suite.TearDownTestSuite = (*Suite)(nil)
)

func (s *Suite) SetupSuite() {
	s.origSwitchToLastUsedWorkspace = switchToLastUsedWorkspace
}

func (s *Suite) TearDownTest() {
	switchToLastUsedWorkspace = s.origSwitchToLastUsedWorkspace
}

func (s *Suite) TestBasicAuth() {
	houstonMock := new(houstonMocks.ClientInterface)
	houstonMock.On("AuthenticateWithBasicAuth", mock.Anything, mock.Anything, mock.Anything).Return(mockToken, nil)

	type args struct {
		username string
		password string
	}
	tests := []struct {
		name         string
		args         args
		want         string
		errAssertion assert.ErrorAssertionFunc
	}{
		{
			name:         "successfully authenticated",
			args:         args{username: "test", password: "test"},
			want:         mockToken,
			errAssertion: assert.NoError,
		},
		{
			name:         "successfully authenticated without password",
			args:         args{username: "test"},
			want:         mockToken,
			errAssertion: assert.NoError,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := basicAuth(tt.args.username, tt.args.password, &config.Context{}, houstonMock)
			if !tt.errAssertion(s.T(), err) {
				return
			}
			s.Equal(tt.want, got)
		})
	}
	houstonMock.AssertExpectations(s.T())
}

func (s *Suite) TestSwitchToLastUsedWorkspace() {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("localhost")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	type args struct {
		c         *config.Context
		workspace *houston.Workspace
		err       error
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "no context set",
			args: args{c: &config.Context{}},
			want: false,
		},
		{
			name: "context set, but no correct workspace set",
			args: args{c: &config.Context{LastUsedWorkspace: "test-workspace-id"}, err: errInvalidWorkspace},
			want: false,
		},
		{
			name: "workspace present, but no domain set",
			args: args{c: &config.Context{LastUsedWorkspace: "test-workspace-id"}, err: errInvalidWorkspace},
			want: false,
		},
		{
			name: "workspace present, with set domain",
			args: args{c: &config.Context{LastUsedWorkspace: "test-workspace-id", Domain: "test-domain"}, workspace: &houston.Workspace{ID: "test-workspace-id"}, err: nil},
			want: true,
		},
		{
			name: "workspace present, unable to set workspace context ",
			args: args{c: &config.Context{LastUsedWorkspace: "test-workspace-id"}, workspace: &houston.Workspace{ID: "test-workspace-id"}, err: nil},
			want: false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			houstonMock := new(houstonMocks.ClientInterface)

			if tt.args.c.LastUsedWorkspace != "" {
				houstonMock.On("ValidateWorkspaceID", tt.args.c.LastUsedWorkspace).Return(tt.args.workspace, tt.args.err).Once()
			}
			got := switchToLastUsedWorkspace(houstonMock, tt.args.c)
			s.Equal(tt.want, got)
		})
	}
}

func (s *Suite) TestRegistryAuthSuccess() {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("localhost")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	mockRegistryHandler := new(mocks.RegistryHandler)
	registryHandlerInit = func(registry string) (airflow.RegistryHandler, error) {
		mockRegistryHandler.On("Login", mock.Anything, mock.Anything).Return(nil).Once()
		return mockRegistryHandler, nil
	}

	out := new(bytes.Buffer)
	houstonMock := new(houstonMocks.ClientInterface)
	houstonMock.On("GetAppConfig", "").Return(&houston.AppConfig{Flags: houston.FeatureFlags{BYORegistryEnabled: false}}, nil)

	tests := []struct {
		name         string
		domain       string
		errAssertion assert.ErrorAssertionFunc
	}{
		{
			name:         "localhost domain",
			domain:       "localhost",
			errAssertion: assert.NoError,
		},
		{
			name:         "houston domain",
			domain:       "houston",
			errAssertion: assert.NoError,
		},
		{
			name:         "localhost platform domain",
			domain:       "localhost.me",
			errAssertion: assert.NoError,
		},
		{
			name:         "platform dev domain",
			domain:       "test.astro.io",
			errAssertion: assert.NoError,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			ctx := config.Context{Domain: tt.domain}
			err := ctx.SetContext()
			s.NoError(err)

			err = ctx.SwitchContext()
			s.NoError(err)

			tt.errAssertion(s.T(), RegistryAuth(houstonMock, out, ""))
		})
	}
	mockRegistryHandler.AssertExpectations(s.T())
}

func (s *Suite) TestRegistryAuthRegistryDomain() {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("localhost")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	tests := []struct {
		name               string
		registryDomain     string
		byoRegistryDomain  string
		byoRegistryEnabled bool
		version            string
	}{
		{
			name:               "custom registry domain with byo registry enabled",
			registryDomain:     "",
			byoRegistryDomain:  "custom.registry.io/astro",
			byoRegistryEnabled: true,
			version:            "0.37.0",
		},
		{
			name:               "custom registry domain with byo registry disabled, v1.0.0",
			registryDomain:     "registry.dp01.astro.io",
			byoRegistryDomain:  "",
			byoRegistryEnabled: false,
			version:            "1.0.0",
		},
		{
			name:               "custom registry domain with byo registry disabled, v0.37.0",
			registryDomain:     "registry.astro.io",
			byoRegistryDomain:  "",
			byoRegistryEnabled: false,
			version:            "0.37.0",
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			ctx := config.Context{Domain: "astro.io"}
			err := ctx.SetContext()
			s.NoError(err)
			err = ctx.SwitchContext()
			s.NoError(err)

			registryDomain := tt.registryDomain
			if tt.byoRegistryEnabled {
				registryDomain, _, _ = strings.Cut(tt.byoRegistryDomain, "/")
			}

			mockRegistryHandler := new(mocks.RegistryHandler)
			mockRegistryHandler.On("Login", mock.Anything, mock.Anything).Return(nil).Once()

			registryHandlerInit = func(registry string) (airflow.RegistryHandler, error) {
				assert.Equal(s.T(), registryDomain, registry)
				return mockRegistryHandler, nil
			}

			houstonMock := new(houstonMocks.ClientInterface)
			houstonMock.On("GetAppConfig", "").Return(&houston.AppConfig{
				Flags: houston.FeatureFlags{
					BYORegistryEnabled: tt.byoRegistryEnabled,
				},
				BYORegistryDomain: tt.byoRegistryDomain,
				Version:           tt.version,
			}, nil)

			out := new(bytes.Buffer)
			RegistryAuth(houstonMock, out, tt.registryDomain)

			mockRegistryHandler.AssertExpectations(s.T())
		})
	}
}

func (s *Suite) TestRegistryAuthFailure() {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig(testUtil.SoftwarePlatform)
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	s.Run("registry failures", func() {
		registryHandlerInit = func(registry string) (airflow.RegistryHandler, error) {
			return nil, errMockRegistry
		}

		out := new(bytes.Buffer)
		houstonMock := new(houstonMocks.ClientInterface)
		houstonMock.On("GetAppConfig", "").Return(&houston.AppConfig{Flags: houston.FeatureFlags{BYORegistryEnabled: true}}, nil).Twice()

		err := RegistryAuth(houstonMock, out, "")
		s.ErrorIs(err, errMockRegistry)

		mockRegistryHandler := new(mocks.RegistryHandler)
		registryHandlerInit = func(registry string) (airflow.RegistryHandler, error) {
			mockRegistryHandler.On("Login", mock.Anything, mock.Anything).Return(errMockRegistry).Once()
			return mockRegistryHandler, nil
		}

		err = RegistryAuth(houstonMock, out, "")
		s.NoError(err)

		houstonMock.On("GetAppConfig", "").Return(&houston.AppConfig{Flags: houston.FeatureFlags{BYORegistryEnabled: false}}, nil).Once()

		err = RegistryAuth(houstonMock, out, "")
		s.ErrorIs(err, errMockRegistry)

		mockRegistryHandler.AssertExpectations(s.T())
		houstonMock.AssertExpectations(s.T())
	})

	s.Run("houston get app config failure", func() {
		out := new(bytes.Buffer)
		houstonMock := new(houstonMocks.ClientInterface)
		houstonMock.On("GetAppConfig", "").Return(nil, errMockHouston).Once()

		err := RegistryAuth(houstonMock, out, "")
		s.ErrorIs(err, errMockHouston)
		houstonMock.AssertExpectations(s.T())
	})
}

func (s *Suite) TestLoginSuccess() {
	s.Run("default without workspace pagination switch", func() {
		fs := afero.NewMemMapFs()
		configYaml := testUtil.NewTestConfig("localhost")
		afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
		config.InitConfig(fs)

		houstonMock := new(houstonMocks.ClientInterface)
		houstonMock.On("GetAuthConfig", mock.Anything).Return(&houston.AuthConfig{LocalEnabled: true}, nil)
		houstonMock.On("AuthenticateWithBasicAuth", mock.Anything, mock.Anything, mock.Anything).Return(mockToken, nil)
		houstonMock.On("ListWorkspaces", nil).Return([]houston.Workspace{{ID: "test-workspace-id"}}, nil).Once()
		houstonMock.On("ValidateWorkspaceID", "test-workspace-id").Return(&houston.Workspace{ID: "test-workspace-id"}, nil).Once()

		out := &bytes.Buffer{}
		if !s.NoError(Login("localhost", false, "test", "test", "0.29.0", houstonMock, out)) {
			return
		}
		if gotOut := out.String(); !testUtil.StringContains([]string{"localhost"}, gotOut) {
			s.Fail("Login() = %v, want %v", gotOut, []string{"localhost"})
		}

		houstonMock.On("ListWorkspaces", nil).Return([]houston.Workspace{{ID: "ck05r3bor07h40d02y2hw4n4v"}, {ID: "test-workspace-id"}}, nil).Once()
		out = &bytes.Buffer{}
		if s.NoError(Login("localhost", false, "test", "test", "0.30.0", houstonMock, out)) {
			return
		}
		if gotOut := out.String(); !testUtil.StringContains([]string{"localhost", "test-workspace-id"}, gotOut) {
			s.Fail("Login() = %v, want %v", gotOut, []string{"localhost", "test-workspace-id"})
		}

		houstonMock.AssertExpectations(s.T())
	})

	s.Run("default with more than one workspace", func() {
		fs := afero.NewMemMapFs()
		configYaml := testUtil.NewTestConfig("localhost")
		afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
		config.InitConfig(fs)
		config.CFG.Interactive.SetHomeString("false")
		config.CFG.PageSize.SetHomeString("100")

		defer testUtil.MockUserInput(s.T(), "1")()

		houstonMock := new(houstonMocks.ClientInterface)
		houstonMock.On("GetAuthConfig", mock.Anything).Return(&houston.AuthConfig{LocalEnabled: true}, nil)
		houstonMock.On("AuthenticateWithBasicAuth", mock.Anything).Return(mockToken, nil)
		houstonMock.On("ListWorkspaces", nil).Return([]houston.Workspace{{ID: "ck05r3bor07h40d02y2hw4n4v"}, {ID: "test-workspace-id"}}, nil).Twice()
		switchToLastUsedWorkspace = func(houstonClient houston.ClientInterface, c *config.Context) bool {
			return false
		}
		houstonMock.On("ValidateWorkspaceID", "ck05r3bor07h40d02y2hw4n4v").Return(&houston.Workspace{}, nil).Once()

		out := &bytes.Buffer{}
		if s.NoError(Login("localhost", false, "test", "test", "0.30.0", houstonMock, out)) {
			return
		}
		if gotOut := out.String(); !testUtil.StringContains([]string{"localhost", "test-workspace-id"}, gotOut) {
			s.Fail("Login() = %v, want %v", gotOut, []string{"localhost", "test-workspace-id"})
		}

		houstonMock.AssertExpectations(s.T())
	})

	s.Run("when interactive set to true, auto selected first workspace if returned one workspace", func() {
		fs := afero.NewMemMapFs()
		configYaml := testUtil.NewTestConfig("localhost")
		afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
		config.InitConfig(fs)
		config.CFG.Interactive.SetHomeString("true")
		config.CFG.PageSize.SetHomeString("100")

		defer testUtil.MockUserInput(s.T(), "1")()

		houstonMock := new(houstonMocks.ClientInterface)
		houstonMock.On("GetAuthConfig", mock.Anything).Return(&houston.AuthConfig{LocalEnabled: true}, nil)
		houstonMock.On("AuthenticateWithBasicAuth", mock.Anything, mock.Anything, mock.Anything).Return(mockToken, nil)
		houstonMock.On("PaginatedListWorkspaces", houston.PaginatedListWorkspaceRequest{PageSize: 2, PageNumber: 0}).Return([]houston.Workspace{{ID: "ck05r3bor07h40d02y2hw4n4v"}}, nil).Once()
		switchToLastUsedWorkspace = func(houstonClient houston.ClientInterface, c *config.Context) bool {
			return false
		}

		out := &bytes.Buffer{}
		if s.NoError(Login("localhost", false, "test", "test", "0.30.0", houstonMock, out)) {
			return
		}
		if gotOut := out.String(); !testUtil.StringContains([]string{"localhost", "ck05r3bor07h40d02y2hw4n4v"}, gotOut) {
			s.Fail("Login() = %v, want %v", gotOut, []string{"localhost", "ck05r3bor07h40d02y2hw4n4v"})
		}

		houstonMock.AssertExpectations(s.T())
	})

	s.Run("when interactive set to true, prompt user to select workspace with pagination", func() {
		fs := afero.NewMemMapFs()
		configYaml := testUtil.NewTestConfig("localhost")
		afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
		config.InitConfig(fs)
		config.CFG.Interactive.SetHomeString("true")
		config.CFG.PageSize.SetHomeString("100")

		defer testUtil.MockUserInput(s.T(), "1")()

		houstonMock := new(houstonMocks.ClientInterface)
		houstonMock.On("GetAuthConfig", mock.Anything).Return(&houston.AuthConfig{LocalEnabled: true}, nil)
		houstonMock.On("AuthenticateWithBasicAuth", mock.Anything).Return(mockToken, nil)
		houstonMock.On("PaginatedListWorkspaces", houston.PaginatedListWorkspaceRequest{PageSize: 2, PageNumber: 0}).Return([]houston.Workspace{{ID: "test-workspace-id"}, {ID: "test-workspace"}}, nil).Once()
		switchToLastUsedWorkspace = func(houstonClient houston.ClientInterface, c *config.Context) bool {
			return false
		}

		houstonMock.On("PaginatedListWorkspaces", houston.PaginatedListWorkspaceRequest{PageSize: 100, PageNumber: 0}).Return([]houston.Workspace{{ID: "test-workspace-1"}, {ID: "test-workspace-2"}}, nil).Once()
		houstonMock.On("ValidateWorkspaceID", "test-workspace-1").Return(&houston.Workspace{}, nil).Once()

		out := &bytes.Buffer{}
		if s.NoError(Login("localhost", false, "test", "test", "0.30.0", houstonMock, out)) {
			return
		}
		if gotOut := out.String(); !testUtil.StringContains([]string{"localhost", "test-workspace-1"}, gotOut) {
			s.Fail("Login() = %v, want %v", gotOut, []string{"localhost", "ck05r3bor07h40d02y2hw4n4v"})
		}

		houstonMock.AssertExpectations(s.T())
	})
}

func (s *Suite) TestLoginFailure() {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("software")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	s.Run("getAuthConfig failure", func() {
		houstonMock := new(houstonMocks.ClientInterface)
		houstonMock.On("GetAuthConfig", mock.Anything).Return(nil, errMockRegistry)

		out := &bytes.Buffer{}
		if !s.ErrorIs(Login("localhost", false, "test", "test", "0.30.0", houstonMock, out), errMockRegistry) {
			return
		}
		if gotOut := out.String(); !testUtil.StringContains([]string{"localhost"}, gotOut) {
			s.Fail("Login() = %v, want %v", gotOut, []string{"localhost"})
		}
		houstonMock.AssertExpectations(s.T())
	})

	s.Run("AuthenticateWithBasicAuth failure", func() {
		houstonMock := new(houstonMocks.ClientInterface)
		houstonMock.On("GetAuthConfig", mock.Anything).Return(&houston.AuthConfig{LocalEnabled: true}, nil)
		houstonMock.On("AuthenticateWithBasicAuth", mock.Anything).Return("", errMockRegistry)

		out := &bytes.Buffer{}
		if s.ErrorIs(Login("localhost", false, "test", "test", "0.30.0", houstonMock, out), errMockRegistry) {
			return
		}
		if gotOut := out.String(); !testUtil.StringContains([]string{"localhost"}, gotOut) {
			s.Fail("Login() = %v, want %v", gotOut, []string{"localhost"})
		}
		houstonMock.AssertExpectations(s.T())
	})

	s.Run("ListWorkspaces failure", func() {
		houstonMock := new(houstonMocks.ClientInterface)
		houstonMock.On("GetAuthConfig", mock.Anything).Return(&houston.AuthConfig{LocalEnabled: true}, nil)
		houstonMock.On("AuthenticateWithBasicAuth", mock.Anything, mock.Anything, mock.Anything).Return(mockToken, nil)
		houstonMock.On("ListWorkspaces", nil).Return([]houston.Workspace{}, errMockRegistry).Once()

		out := &bytes.Buffer{}
		if s.ErrorIs(Login("localhost", false, "test", "test", "0.30.0", houstonMock, out), errMockRegistry) {
			return
		}
		if gotOut := out.String(); !testUtil.StringContains([]string{"localhost"}, gotOut) {
			s.Fail("Login() = %v, want %v", gotOut, []string{"localhost"})
		}
		houstonMock.AssertExpectations(s.T())
	})

	s.Run("no workspace failure", func() {
		houstonMock := new(houstonMocks.ClientInterface)
		houstonMock.On("GetAuthConfig", mock.Anything).Return(&houston.AuthConfig{LocalEnabled: true}, nil)
		houstonMock.On("AuthenticateWithBasicAuth", mock.Anything).Return(mockToken, nil)
		houstonMock.On("ListWorkspaces", nil).Return([]houston.Workspace{{ID: "ck05r3bor07h40d02y2hw4n4v"}, {ID: "test-workspace-id"}}, nil)
		houstonMock.On("GetAppConfig", "").Return(&houston.AppConfig{Flags: houston.FeatureFlags{BYORegistryEnabled: false}}, nil)

		out := &bytes.Buffer{}
		if s.NoError(Login("dev.astro.io", false, "test", "test", "0.30.0", houstonMock, out)) {
			return
		}
		if gotOut := out.String(); !testUtil.StringContains([]string{"dev.astro.io", "No default workspace detected"}, gotOut) {
			s.Fail("Login() = %v, want %v", gotOut, []string{"dev.astro.io", "No default workspace detected"})
		}
		houstonMock.AssertExpectations(s.T())
	})

	s.Run("registry login failure", func() {
		houstonMock := new(houstonMocks.ClientInterface)
		houstonMock.On("GetAuthConfig", mock.Anything).Return(&houston.AuthConfig{LocalEnabled: true}, nil)
		houstonMock.On("AuthenticateWithBasicAuth", mock.Anything, mock.Anything, mock.Anything).Return(mockToken, nil)
		houstonMock.On("ListWorkspaces", nil).Return([]houston.Workspace{{ID: "test-workspace-id"}}, nil).Once()
		houstonMock.On("GetAppConfig", "").Return(&houston.AppConfig{Flags: houston.FeatureFlags{BYORegistryEnabled: false}}, nil)

		mockRegistryHandler := new(mocks.RegistryHandler)
		registryHandlerInit = func(registry string) (airflow.RegistryHandler, error) {
			mockRegistryHandler.On("Login", mock.Anything, mock.Anything).Return(errMockRegistry).Once()
			return mockRegistryHandler, nil
		}

		out := &bytes.Buffer{}
		if s.NoError(Login("test.astro.io", false, "test", "test", "0.30.0", houstonMock, out)) {
			return
		}
		if gotOut := out.String(); !testUtil.StringContains([]string{"test.astro.io", "Failed to authenticate to the registry"}, gotOut) {
			s.Fail("Login() = %v, want %v", gotOut, []string{"test.astro.io", "Failed to authenticate to the registry"})
		}
		houstonMock.AssertExpectations(s.T())
	})
}

func (s *Suite) TestLogout() {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("localhost")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	type args struct {
		domain string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "logout domain, not present in contexts",
			args: args{domain: "test.astro.io"},
		},
		{
			name: "logout localhost",
			args: args{domain: "localhost"},
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			Logout(tt.args.domain)
		})
	}
}

func (s *Suite) TestCheckClusterDomain() {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("localhost")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	type args struct {
		domain string
	}
	tests := []struct {
		name         string
		args         args
		errAssertion assert.ErrorAssertionFunc
	}{
		{
			name:         "new domain, not present in existing domains",
			args:         args{domain: "test.astro.io"},
			errAssertion: assert.NoError,
		},
		{
			name:         "switch to existing domain",
			args:         args{domain: "localhost"},
			errAssertion: assert.NoError,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			tt.errAssertion(s.T(), checkClusterDomain(tt.args.domain))
		})
	}
}

func (s *Suite) TestGetAuthTokenSuccess() {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("software")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	houstonMock := new(houstonMocks.ClientInterface)
	houstonMock.On("AuthenticateWithBasicAuth", mock.Anything).Return(mockToken, nil)

	type args struct {
		username   string
		password   string
		authConfig *houston.AuthConfig
		ctx        *config.Context
	}
	tests := []struct {
		name         string
		args         args
		want         string
		errAssertion assert.ErrorAssertionFunc
	}{
		{
			name:         "basic test with user & pass",
			args:         args{username: "test", password: "test", authConfig: &houston.AuthConfig{LocalEnabled: true}, ctx: &config.Context{Domain: "localhost"}},
			want:         mockToken,
			errAssertion: assert.NoError,
		},
		{
			name:         "basic test with oauth token",
			args:         args{username: "", password: "", authConfig: &houston.AuthConfig{LocalEnabled: true, AuthProviders: []houston.AuthProvider{{Name: "oAuth-test-provider"}}}, ctx: &config.Context{Domain: "localhost"}},
			want:         "",
			errAssertion: assert.NoError,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := getAuthToken(tt.args.username, tt.args.password, tt.args.authConfig, tt.args.ctx, houstonMock)
			if !tt.errAssertion(s.T(), err) {
				return
			}
			if got != tt.want {
				s.Fail("getAuthToken() = %v, want %v", got, tt.want)
			}
		})
	}

	houstonMock.AssertExpectations(s.T())
}

func (s *Suite) TestGetAuthTokenFailure() {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("software")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	got, err := getAuthToken("", "", &houston.AuthConfig{LocalEnabled: true}, &config.Context{Domain: "localhost"}, nil)
	if !s.ErrorIs(err, errOAuthDisabled) {
		return
	}
	s.Equal(got, "")

	houstonMock := new(houstonMocks.ClientInterface)
	houstonMock.On("AuthenticateWithBasicAuth", mock.Anything).Return("", errMockRegistry)

	_, err = getAuthToken("test", "test", &houston.AuthConfig{LocalEnabled: true}, &config.Context{Domain: "localhost"}, houstonMock)
	if !s.ErrorIs(err, errMockRegistry) {
		return
	}
	houstonMock.AssertExpectations(s.T())
}
