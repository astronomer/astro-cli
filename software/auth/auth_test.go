package auth

import (
	"bytes"
	"errors"
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
)

const (
	mockToken = "token"
)

var (
	errMockRegistry     = errors.New("some error on registry side")
	errMockHouston      = errors.New("some error on houston side")
	errInvalidWorkspace = errors.New("last used workspace id is not valid")
)

func TestBasicAuth(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
			got, err := basicAuth(tt.args.username, tt.args.password, &config.Context{}, houstonMock)
			if !tt.errAssertion(t, err) {
				return
			}
			if got != tt.want {
				t.Errorf("basicAuth() = %v, want %v", got, tt.want)
			}
		})
	}
	houstonMock.AssertExpectations(t)
}

func TestSwitchToLastUsedWorkspace(t *testing.T) {
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

	houstonMock := new(houstonMocks.ClientInterface)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.args.c.LastUsedWorkspace != "" {
				houstonMock.On("ValidateWorkspaceID", tt.args.c.LastUsedWorkspace).Return(tt.args.workspace, tt.args.err).Once()
			}
			if got := switchToLastUsedWorkspace(houstonMock, tt.args.c); got != tt.want {
				t.Errorf("switchToLastUsedWorkspace() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRegistryAuthSuccess(t *testing.T) {
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
	houstonMock.On("GetAppConfig", nil).Return(&houston.AppConfig{Flags: houston.FeatureFlags{BYORegistryEnabled: false}}, nil)

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
		t.Run(tt.name, func(t *testing.T) {
			ctx := config.Context{Domain: tt.domain}
			err := ctx.SetContext()
			assert.NoError(t, err)

			err = ctx.SwitchContext()
			assert.NoError(t, err)

			tt.errAssertion(t, registryAuth(houstonMock, out))
		})
	}
	mockRegistryHandler.AssertExpectations(t)
}

func TestRegistryAuthFailure(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig(testUtil.SoftwarePlatform)
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	t.Run("registry failures", func(t *testing.T) {
		registryHandlerInit = func(registry string) (airflow.RegistryHandler, error) {
			return nil, errMockRegistry
		}

		out := new(bytes.Buffer)
		houstonMock := new(houstonMocks.ClientInterface)
		houstonMock.On("GetAppConfig", nil).Return(&houston.AppConfig{Flags: houston.FeatureFlags{BYORegistryEnabled: true}}, nil).Twice()

		err := registryAuth(houstonMock, out)
		assert.ErrorIs(t, err, errMockRegistry)

		mockRegistryHandler := new(mocks.RegistryHandler)
		registryHandlerInit = func(registry string) (airflow.RegistryHandler, error) {
			mockRegistryHandler.On("Login", mock.Anything, mock.Anything).Return(errMockRegistry).Once()
			return mockRegistryHandler, nil
		}

		err = registryAuth(houstonMock, out)
		assert.NoError(t, err)

		houstonMock.On("GetAppConfig", nil).Return(&houston.AppConfig{Flags: houston.FeatureFlags{BYORegistryEnabled: false}}, nil).Once()

		err = registryAuth(houstonMock, out)
		assert.ErrorIs(t, err, errMockRegistry)

		mockRegistryHandler.AssertExpectations(t)
		houstonMock.AssertExpectations(t)
	})

	t.Run("houston get app config failure", func(t *testing.T) {
		out := new(bytes.Buffer)
		houstonMock := new(houstonMocks.ClientInterface)
		houstonMock.On("GetAppConfig", nil).Return(nil, errMockHouston).Once()

		err := registryAuth(houstonMock, out)
		assert.ErrorIs(t, err, errMockHouston)
		houstonMock.AssertExpectations(t)
	})
}

func TestLoginSuccess(t *testing.T) {
	t.Run("default without workspace pagination switch", func(t *testing.T) {
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
		if !assert.NoError(t, Login("localhost", false, "test", "test", "0.29.0", houstonMock, out)) {
			return
		}
		if gotOut := out.String(); !testUtil.StringContains([]string{"localhost"}, gotOut) {
			t.Errorf("Login() = %v, want %v", gotOut, []string{"localhost"})
		}

		houstonMock.On("ListWorkspaces", nil).Return([]houston.Workspace{{ID: "ck05r3bor07h40d02y2hw4n4v"}, {ID: "test-workspace-id"}}, nil).Once()
		out = &bytes.Buffer{}
		if assert.NoError(t, Login("localhost", false, "test", "test", "0.30.0", houstonMock, out)) {
			return
		}
		if gotOut := out.String(); !testUtil.StringContains([]string{"localhost", "test-workspace-id"}, gotOut) {
			t.Errorf("Login() = %v, want %v", gotOut, []string{"localhost", "test-workspace-id"})
		}

		houstonMock.AssertExpectations(t)
	})

	t.Run("default with more than one workspace", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		configYaml := testUtil.NewTestConfig("localhost")
		afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
		config.InitConfig(fs)
		config.CFG.Interactive.SetHomeString("false")
		config.CFG.PageSize.SetHomeString("100")

		defer testUtil.MockUserInput(t, "1")()

		houstonMock := new(houstonMocks.ClientInterface)
		houstonMock.On("GetAuthConfig", mock.Anything).Return(&houston.AuthConfig{LocalEnabled: true}, nil)
		houstonMock.On("AuthenticateWithBasicAuth", mock.Anything).Return(mockToken, nil)
		houstonMock.On("ListWorkspaces", nil).Return([]houston.Workspace{{ID: "ck05r3bor07h40d02y2hw4n4v"}, {ID: "test-workspace-id"}}, nil).Twice()
		switchToLastUsedWorkspace = func(houstonClient houston.ClientInterface, c *config.Context) bool {
			return false
		}
		houstonMock.On("ValidateWorkspaceID", "ck05r3bor07h40d02y2hw4n4v").Return(&houston.Workspace{}, nil).Once()

		out := &bytes.Buffer{}
		if assert.NoError(t, Login("localhost", false, "test", "test", "0.30.0", houstonMock, out)) {
			return
		}
		if gotOut := out.String(); !testUtil.StringContains([]string{"localhost", "test-workspace-id"}, gotOut) {
			t.Errorf("Login() = %v, want %v", gotOut, []string{"localhost", "test-workspace-id"})
		}

		houstonMock.AssertExpectations(t)
	})

	t.Run("when interactive set to true, auto selected first workspace if returned one workspace", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		configYaml := testUtil.NewTestConfig("localhost")
		afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
		config.InitConfig(fs)
		config.CFG.Interactive.SetHomeString("true")
		config.CFG.PageSize.SetHomeString("100")

		defer testUtil.MockUserInput(t, "1")()

		houstonMock := new(houstonMocks.ClientInterface)
		houstonMock.On("GetAuthConfig", mock.Anything).Return(&houston.AuthConfig{LocalEnabled: true}, nil)
		houstonMock.On("AuthenticateWithBasicAuth", mock.Anything, mock.Anything, mock.Anything).Return(mockToken, nil)
		houstonMock.On("PaginatedListWorkspaces", houston.PaginatedListWorkspaceRequest{PageSize: 2, PageNumber: 0}).Return([]houston.Workspace{{ID: "ck05r3bor07h40d02y2hw4n4v"}}, nil).Once()
		switchToLastUsedWorkspace = func(houstonClient houston.ClientInterface, c *config.Context) bool {
			return false
		}

		out := &bytes.Buffer{}
		if assert.NoError(t, Login("localhost", false, "test", "test", "0.30.0", houstonMock, out)) {
			return
		}
		if gotOut := out.String(); !testUtil.StringContains([]string{"localhost", "ck05r3bor07h40d02y2hw4n4v"}, gotOut) {
			t.Errorf("Login() = %v, want %v", gotOut, []string{"localhost", "ck05r3bor07h40d02y2hw4n4v"})
		}

		houstonMock.AssertExpectations(t)
	})

	t.Run("when interactive set to true, prompt user to select workspace with pagination", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		configYaml := testUtil.NewTestConfig("localhost")
		afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
		config.InitConfig(fs)
		config.CFG.Interactive.SetHomeString("true")
		config.CFG.PageSize.SetHomeString("100")

		defer testUtil.MockUserInput(t, "1")()

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
		if assert.NoError(t, Login("localhost", false, "test", "test", "0.30.0", houstonMock, out)) {
			return
		}
		if gotOut := out.String(); !testUtil.StringContains([]string{"localhost", "test-workspace-1"}, gotOut) {
			t.Errorf("Login() = %v, want %v", gotOut, []string{"localhost", "ck05r3bor07h40d02y2hw4n4v"})
		}

		houstonMock.AssertExpectations(t)
	})
}

func TestLoginFailure(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("software")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	t.Run("getAuthConfig failure", func(t *testing.T) {
		houstonMock := new(houstonMocks.ClientInterface)
		houstonMock.On("GetAuthConfig", mock.Anything).Return(nil, errMockRegistry)

		out := &bytes.Buffer{}
		if !assert.ErrorIs(t, Login("localhost", false, "test", "test", "0.30.0", houstonMock, out), errMockRegistry) {
			return
		}
		if gotOut := out.String(); !testUtil.StringContains([]string{"localhost"}, gotOut) {
			t.Errorf("Login() = %v, want %v", gotOut, []string{"localhost"})
		}
		houstonMock.AssertExpectations(t)
	})

	t.Run("AuthenticateWithBasicAuth failure", func(t *testing.T) {
		houstonMock := new(houstonMocks.ClientInterface)
		houstonMock.On("GetAuthConfig", mock.Anything).Return(&houston.AuthConfig{LocalEnabled: true}, nil)
		houstonMock.On("AuthenticateWithBasicAuth", mock.Anything).Return("", errMockRegistry)

		out := &bytes.Buffer{}
		if assert.ErrorIs(t, Login("localhost", false, "test", "test", "0.30.0", houstonMock, out), errMockRegistry) {
			return
		}
		if gotOut := out.String(); !testUtil.StringContains([]string{"localhost"}, gotOut) {
			t.Errorf("Login() = %v, want %v", gotOut, []string{"localhost"})
		}
		houstonMock.AssertExpectations(t)
	})

	t.Run("ListWorkspaces failure", func(t *testing.T) {
		houstonMock := new(houstonMocks.ClientInterface)
		houstonMock.On("GetAuthConfig", mock.Anything).Return(&houston.AuthConfig{LocalEnabled: true}, nil)
		houstonMock.On("AuthenticateWithBasicAuth", mock.Anything, mock.Anything, mock.Anything).Return(mockToken, nil)
		houstonMock.On("ListWorkspaces", nil).Return([]houston.Workspace{}, errMockRegistry).Once()

		out := &bytes.Buffer{}
		if assert.ErrorIs(t, Login("localhost", false, "test", "test", "0.30.0", houstonMock, out), errMockRegistry) {
			return
		}
		if gotOut := out.String(); !testUtil.StringContains([]string{"localhost"}, gotOut) {
			t.Errorf("Login() = %v, want %v", gotOut, []string{"localhost"})
		}
		houstonMock.AssertExpectations(t)
	})

	t.Run("no workspace failure", func(t *testing.T) {
		houstonMock := new(houstonMocks.ClientInterface)
		houstonMock.On("GetAuthConfig", mock.Anything).Return(&houston.AuthConfig{LocalEnabled: true}, nil)
		houstonMock.On("AuthenticateWithBasicAuth", mock.Anything).Return(mockToken, nil)
		houstonMock.On("ListWorkspaces", nil).Return([]houston.Workspace{{ID: "ck05r3bor07h40d02y2hw4n4v"}, {ID: "test-workspace-id"}}, nil)
		houstonMock.On("GetAppConfig", nil).Return(&houston.AppConfig{Flags: houston.FeatureFlags{BYORegistryEnabled: false}}, nil)

		out := &bytes.Buffer{}
		if assert.NoError(t, Login("dev.astro.io", false, "test", "test", "0.30.0", houstonMock, out)) {
			return
		}
		if gotOut := out.String(); !testUtil.StringContains([]string{"dev.astro.io", "No default workspace detected"}, gotOut) {
			t.Errorf("Login() = %v, want %v", gotOut, []string{"dev.astro.io", "No default workspace detected"})
		}
		houstonMock.AssertExpectations(t)
	})

	t.Run("registry login failure", func(t *testing.T) {
		houstonMock := new(houstonMocks.ClientInterface)
		houstonMock.On("GetAuthConfig", mock.Anything).Return(&houston.AuthConfig{LocalEnabled: true}, nil)
		houstonMock.On("AuthenticateWithBasicAuth", mock.Anything, mock.Anything, mock.Anything).Return(mockToken, nil)
		houstonMock.On("ListWorkspaces", nil).Return([]houston.Workspace{{ID: "test-workspace-id"}}, nil).Once()
		houstonMock.On("GetAppConfig", nil).Return(&houston.AppConfig{Flags: houston.FeatureFlags{BYORegistryEnabled: false}}, nil)

		mockRegistryHandler := new(mocks.RegistryHandler)
		registryHandlerInit = func(registry string) (airflow.RegistryHandler, error) {
			mockRegistryHandler.On("Login", mock.Anything, mock.Anything).Return(errMockRegistry).Once()
			return mockRegistryHandler, nil
		}

		out := &bytes.Buffer{}
		if assert.NoError(t, Login("test.astro.io", false, "test", "test", "0.30.0", houstonMock, out)) {
			return
		}
		if gotOut := out.String(); !testUtil.StringContains([]string{"test.astro.io", "Failed to authenticate to the registry"}, gotOut) {
			t.Errorf("Login() = %v, want %v", gotOut, []string{"test.astro.io", "Failed to authenticate to the registry"})
		}
		houstonMock.AssertExpectations(t)
	})
}

func TestLogout(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
			Logout(tt.args.domain)
		})
	}
}

func TestCheckClusterDomain(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
			tt.errAssertion(t, checkClusterDomain(tt.args.domain))
		})
	}
}

func TestGetAuthTokenSuccess(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
			got, err := getAuthToken(tt.args.username, tt.args.password, tt.args.authConfig, tt.args.ctx, houstonMock)
			if !tt.errAssertion(t, err) {
				return
			}
			if got != tt.want {
				t.Errorf("getAuthToken() = %v, want %v", got, tt.want)
			}
		})
	}

	houstonMock.AssertExpectations(t)
}

func TestGetAuthTokenFailure(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("software")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	got, err := getAuthToken("", "", &houston.AuthConfig{LocalEnabled: true}, &config.Context{Domain: "localhost"}, nil)
	if !assert.ErrorIs(t, err, errOAuthDisabled) {
		return
	}
	assert.Equal(t, got, "")

	houstonMock := new(houstonMocks.ClientInterface)
	houstonMock.On("AuthenticateWithBasicAuth", mock.Anything).Return("", errMockRegistry)

	_, err = getAuthToken("test", "test", &houston.AuthConfig{LocalEnabled: true}, &config.Context{Domain: "localhost"}, houstonMock)
	if !assert.ErrorIs(t, err, errMockRegistry) {
		return
	}
	houstonMock.AssertExpectations(t)
}
