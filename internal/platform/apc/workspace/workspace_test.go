package workspace

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	"github.com/astronomer/astro-cli/pkg/input"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var (
	mockWorkspace     *houston.Workspace
	mockWorkspaceList []houston.Workspace
)

type Suite struct {
	suite.Suite
}

var (
	_ suite.SetupAllSuite     = (*Suite)(nil)
	_ suite.TearDownTestSuite = (*Suite)(nil)
)

func (s *Suite) SetupSuite() {
	mockWorkspace = &houston.Workspace{
		ID:           "ckc0j8y1101xo0760or02jdi7",
		Label:        "test",
		Description:  "description",
		Users:        nil,
		CreatedAt:    "",
		UpdatedAt:    "",
		RoleBindings: nil,
	}
	mockWorkspaceList = []houston.Workspace{
		{
			ID:           "ckbv7zvb100pe0760xp98qnh9",
			Label:        "w1",
			Description:  "description",
			Users:        nil,
			CreatedAt:    "",
			UpdatedAt:    "",
			RoleBindings: nil,
		},
		{
			ID:           "ckbv8pwbq00wk0760us7ktcgd",
			Label:        "wwww",
			Description:  "description",
			Users:        nil,
			CreatedAt:    "",
			UpdatedAt:    "",
			RoleBindings: nil,
		},
		{
			ID:           "ckc0j8y1101xo0760or02jdi7",
			Label:        "test",
			Description:  "description",
			Users:        nil,
			CreatedAt:    "",
			UpdatedAt:    "",
			RoleBindings: nil,
		},
	}
}

func (s *Suite) TearDownTest() {
}

func TestWorkspace(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestCreate() {
	testUtil.InitTestConfig("software")

	label := "test"
	description := "description"

	api := new(mocks.ClientInterface)
	api.On("CreateWorkspace", houston.CreateWorkspaceRequest{Label: label, Description: description}).Return(mockWorkspace, nil)

	buf := new(bytes.Buffer)
	err := Create(label, description, api, buf)
	s.NoError(err)
	expected := ` NAME     ID                            
 test     ckc0j8y1101xo0760or02jdi7     

 Successfully created workspace
`
	s.Equal(buf.String(), expected)
	api.AssertExpectations(s.T())
}

func (s *Suite) TestCreateError() {
	testUtil.InitTestConfig("software")

	label := "test"
	description := "description"

	api := new(mocks.ClientInterface)
	api.On("CreateWorkspace", houston.CreateWorkspaceRequest{Label: label, Description: description}).Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := Create(label, description, api, buf)
	s.EqualError(err, errMock.Error())
	api.AssertExpectations(s.T())
}

func (s *Suite) TestList() {
	testUtil.InitTestConfig("software")

	api := new(mocks.ClientInterface)
	api.On("ListWorkspaces", nil).Return(mockWorkspaceList, nil)

	buf := new(bytes.Buffer)
	err := List(api, buf)
	s.NoError(err)
	expected := ` NAME     ID                            
 w1       ckbv7zvb100pe0760xp98qnh9     
 wwww     ckbv8pwbq00wk0760us7ktcgd     
 test     ckc0j8y1101xo0760or02jdi7     
`
	s.Equal(buf.String(), expected)
	api.AssertExpectations(s.T())
}

func (s *Suite) TestListActiveWorkspace() {
	testUtil.InitTestConfig("software")

	// Add active workspace to mocks
	mockWorkspaceList[0].ID = "ck05r3bor07h40d02y2hw4n4v"

	api := new(mocks.ClientInterface)
	api.On("ListWorkspaces", nil).Return(mockWorkspaceList, nil)

	buf := new(bytes.Buffer)
	err := List(api, buf)
	s.NoError(err)
	expected := " NAME     ID                            \n\x1b[1;32m w1       ck05r3bor07h40d02y2hw4n4v     \x1b[0m\n wwww     ckbv8pwbq00wk0760us7ktcgd     \n test     ckc0j8y1101xo0760or02jdi7     \n"
	s.Equal(expected, buf.String())
	api.AssertExpectations(s.T())
}

func (s *Suite) TestListError() {
	testUtil.InitTestConfig("software")

	api := new(mocks.ClientInterface)
	api.On("ListWorkspaces", nil).Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := List(api, buf)
	s.EqualError(err, errMock.Error())
	api.AssertExpectations(s.T())
}

func (s *Suite) TestDelete() {
	testUtil.InitTestConfig("software")

	mockResponse := &houston.Workspace{
		ID: "ckc0j8y1101xo0760or02jdi7",
	}

	api := new(mocks.ClientInterface)
	api.On("DeleteWorkspace", mockResponse.ID).Return(mockResponse, nil)

	buf := new(bytes.Buffer)
	err := Delete(mockResponse.ID, api, buf)
	s.NoError(err)
	expected := "\n Successfully deleted workspace\n"
	s.Equal(expected, buf.String())
	api.AssertExpectations(s.T())
}

func (s *Suite) TestDeleteError() {
	testUtil.InitTestConfig("software")

	wsID := "ckc0j8y1101xo0760or02jdi7"

	api := new(mocks.ClientInterface)
	api.On("DeleteWorkspace", wsID).Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := Delete(wsID, api, buf)
	s.EqualError(err, errMock.Error())
	api.AssertExpectations(s.T())
}

func (s *Suite) TestGetWorkspaceSelectionId() {
	// Create a mock client
	testUtil.InitTestConfig("software")
	api := new(mocks.ClientInterface)
	api.On("ListWorkspaces", "test-org-id").Return([]houston.Workspace{
		{ID: "123", Label: "Workspace 1"},
		{ID: "456", Label: "Workspace 2"},
	}, nil)

	// Set up a mock output buffer
	buf := new(bytes.Buffer)

	// Set up mock input
	testUtil.MockUserInput(s.T(), "1\n")
	// Call the function
	workspaceID, err := GetWorkspaceSelectionID(api, buf)
	if err != nil {
		s.Fail("Unexpected error: %s", err)
	}

	// Check the output buffer: the numbered rows, then the prompt.
	lines := strings.Split(buf.String(), "\n")
	s.Equal([]string{"#", "NAME", "ID"}, strings.Fields(lines[0]))
	s.Equal([]string{"1", "Workspace", "1", "123"}, strings.Fields(lines[1]))
	s.Equal([]string{"2", "Workspace", "2", "456"}, strings.Fields(lines[2]))
	s.True(strings.HasSuffix(buf.String(), "\n\n> "), buf.String())

	// Check the selected workspace ID
	s.Equal("123", workspaceID)

	testUtil.MockUserInput(s.T(), "7\n")
	// Call the function
	_, err = GetWorkspaceSelectionID(api, buf)
	s.ErrorIs(err, errInvalidWorkspaceKey)
}

func (s *Suite) TestGetCurrentWorkspace() {
	// we init default workspace to: ck05r3bor07h40d02y2hw4n4v
	testUtil.InitTestConfig("software")

	ws, err := GetCurrentWorkspace()
	s.NoError(err)
	s.Equal("ck05r3bor07h40d02y2hw4n4v", ws)
}

func (s *Suite) TestGetCurrentWorkspaceError() {
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, config.HomeConfigFile, []byte(""), 0o777)
	config.InitConfig(fs)
	_, err := GetCurrentWorkspace()
	s.EqualError(err, "no context set, have you authenticated to Astro or APC? Run astro login and try again")
}

func (s *Suite) TestGetCurrentWorkspaceErrorNoCurrentContext() {
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
context: localhost
contexts:
  localhost:
    domain: localhost
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace:
`)
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, config.HomeConfigFile, configRaw, 0o777)
	config.InitConfig(fs)
	_, err := GetCurrentWorkspace()
	s.EqualError(err, "current workspace context not set, you can switch to a workspace with \n\tastro workspace switch WORKSPACEID")
}

func (s *Suite) TestGetWorkspaceSelectionError() {
	testUtil.InitTestConfig("software")

	api := new(mocks.ClientInterface)
	api.On("ListWorkspaces", nil).Return(nil, errMock)

	buf := new(bytes.Buffer)
	workspaceSelection := getWorkspaceSelection(0, 0, api, buf)
	s.EqualError(workspaceSelection.err, errMock.Error())
	api.AssertExpectations(s.T())
}

func (s *Suite) TestSwitch() {
	// prepare test config and init it
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
context: localhost
contexts:
  localhost:
    domain: localhost
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace:
`)
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, config.HomeConfigFile, configRaw, 0o777)
	config.InitConfig(fs)

	api := new(mocks.ClientInterface)
	api.On("ValidateWorkspaceID", mockWorkspace.ID).Return(mockWorkspace, nil)
	api.On("ListWorkspaces", nil).Return(mockWorkspaceList, nil)

	defer testUtil.MockUserInput(s.T(), "3")()

	buf := new(bytes.Buffer)
	err := Switch("", 0, api, buf)
	s.NoError(err)
	s.Contains(buf.String(), mockWorkspace.ID)
	api.AssertExpectations(s.T())
}

func (s *Suite) TestSwitchWithQuitSelection() {
	// prepare test config and init it
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
context: localhost
contexts:
  localhost:
    domain: localhost
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace:
`)
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, config.HomeConfigFile, configRaw, 0o777)
	config.InitConfig(fs)

	api := new(mocks.ClientInterface)
	api.On("PaginatedListWorkspaces", houston.PaginatedListWorkspaceRequest{PageSize: 10, PageNumber: 0}).Return(mockWorkspaceList, nil)

	defer testUtil.MockUserInput(s.T(), "q")()

	buf := new(bytes.Buffer)
	err := Switch("", 10, api, buf)
	s.NoError(err)
	api.AssertExpectations(s.T())
}

func (s *Suite) TestSwitchWithError() {
	// prepare test config and init it
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
context: localhost
contexts:
  localhost:
    domain: localhost
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace:
`)
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, config.HomeConfigFile, configRaw, 0o777)
	config.InitConfig(fs)

	api := new(mocks.ClientInterface)
	api.On("ListWorkspaces", nil).Return(mockWorkspaceList, nil)

	defer testUtil.MockUserInput(s.T(), "y")()

	buf := new(bytes.Buffer)
	err := Switch("", 0, api, buf)
	s.ErrorIs(err, errInvalidWorkspaceKey)
	s.Contains(buf.String(), mockWorkspace.ID)
	api.AssertExpectations(s.T())
}

func (s *Suite) TestSwitchHoustonError() {
	// prepare test config and init it
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
context: localhost
contexts:
  localhost:
    domain: localhost
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace:
`)
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, config.HomeConfigFile, configRaw, 0o777)
	config.InitConfig(fs)

	wsID := "ckbv7zvb100pe0760xp98qnh9"

	api := new(mocks.ClientInterface)
	api.On("ValidateWorkspaceID", wsID).Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := Switch(wsID, 0, api, buf)
	s.EqualError(err, "workspace id is not valid: api error")
	api.AssertExpectations(s.T())
}

func (s *Suite) TestUpdate() {
	testUtil.InitTestConfig("software")

	id := "test"
	args := map[string]string{"1": "2"}

	api := new(mocks.ClientInterface)
	api.On("UpdateWorkspace", houston.UpdateWorkspaceRequest{WorkspaceID: id, Args: args}).Return(mockWorkspace, nil)

	buf := new(bytes.Buffer)
	err := Update(id, api, buf, args)
	s.NoError(err)
	expected := " NAME     ID                            \n test     ckc0j8y1101xo0760or02jdi7     \n\n Successfully updated workspace\n"
	s.Equal(expected, buf.String())
	api.AssertExpectations(s.T())
}

func (s *Suite) TestUpdateError() {
	testUtil.InitTestConfig("software")

	// prepare houston-api fake response
	id := "test"
	args := map[string]string{"1": "2"}

	api := new(mocks.ClientInterface)
	api.On("UpdateWorkspace", houston.UpdateWorkspaceRequest{WorkspaceID: id, Args: args}).Return(nil, errMock)

	buf := new(bytes.Buffer)
	err := Update(id, api, buf, args)
	s.EqualError(err, errMock.Error())
	api.AssertExpectations(s.T())
}

func (s *Suite) TestGetWorkspaceSelection() {
	api := new(mocks.ClientInterface)
	api.On("ListWorkspaces", nil).Return(mockWorkspaceList, nil)
	api.On("PaginatedListWorkspaces", houston.PaginatedListWorkspaceRequest{PageSize: 10, PageNumber: 0}).Return(mockWorkspaceList, nil)

	s.Run("no context set", func() {
		err := config.ResetCurrentContext()
		s.NoError(err)
		out := new(bytes.Buffer)
		workspaceSelection := getWorkspaceSelection(0, 0, api, out)

		s.Contains(workspaceSelection.err.Error(), "no context set, have you authenticated to Astro or APC? Run astro login and try again")
		s.Equal("", workspaceSelection.id)
	})

	testUtil.InitTestConfig("software")

	s.Run("success", func() {
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(s.T(), "1")()
		workspaceSelection := getWorkspaceSelection(0, 0, api, out)

		s.NoError(workspaceSelection.err)
		s.Equal("ckbv7zvb100pe0760xp98qnh9", workspaceSelection.id)
	})

	s.Run("success with pagination", func() {
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(s.T(), "1")()
		workspaceSelection := getWorkspaceSelection(10, 0, api, out)

		s.NoError(workspaceSelection.err)
		s.Equal("ckbv7zvb100pe0760xp98qnh9", workspaceSelection.id)
	})

	s.Run("invalid selection", func() {
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(s.T(), "y")()
		workspaceSelection := getWorkspaceSelection(0, 0, api, out)

		s.ErrorIs(workspaceSelection.err, errInvalidWorkspaceKey)
		s.Equal("", workspaceSelection.id)
	})

	s.Run("quit selection when paginated", func() {
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(s.T(), "q")()
		workspaceSelection := getWorkspaceSelection(10, 0, api, out)
		s.Nil(workspaceSelection.err)
		s.Equal("", workspaceSelection.id)
		s.Equal(true, workspaceSelection.quit)
	})
}

func (s *Suite) TestWorkspacesPromptPaginatedOption() {
	s.Run("quit selection when total record less then page size and page first", func() {
		defer testUtil.MockUserInput(s.T(), "q")()
		resp, err := workspacesPromptPaginatedOption(3, 0, 3)
		s.NoError(err)
		expected := workspacePaginationOptions{pageSize: 3, pageNumber: 0, quit: true, userSelection: 0}

		s.Equal(expected, resp)
	})
}

// A row number that names no row, or names one in any spelling but plain
// decimal, picks nothing and fails with the invalid-selection error, whether
// the list is paged or not. Before, 0 or a number past the end indexed the
// fetched rows out of range and panicked, "01" and "+1" picked row 1, and the
// paged prompt asked again on a closed stdin forever.
func (s *Suite) TestGetWorkspaceSelectionRefusesAnythingButARowNumber() {
	testUtil.InitTestConfig("software")
	api := new(mocks.ClientInterface)
	api.On("ListWorkspaces", nil).Return(mockWorkspaceList, nil)
	api.On("PaginatedListWorkspaces", houston.PaginatedListWorkspaceRequest{PageSize: 10, PageNumber: 0}).Return(mockWorkspaceList, nil)
	api.On("PaginatedListWorkspaces", houston.PaginatedListWorkspaceRequest{PageSize: 3, PageNumber: 1}).Return(mockWorkspaceList, nil)

	for _, tc := range []struct {
		name                 string
		pageSize, pageNumber int
		answers              []string
	}{
		{"not paged", 0, 0, []string{"0", "4", "-1", "01", "+1", " 1", "1.0", "y", ""}},
		{"paged, first page", 10, 0, []string{"0", "4", "-1", "01", "+1", " 1", "1.0", "y", ""}},
		// The second page of three shows rows 4 to 6; 1 to 3 are the page before.
		{"paged, second page", 3, 1, []string{"1", "3", "7", "04", "+4"}},
	} {
		for _, answer := range tc.answers {
			s.Run(tc.name+" "+answer, func() {
				defer testUtil.MockUserInput(s.T(), answer+"\n")()
				got := getWorkspaceSelection(tc.pageSize, tc.pageNumber, api, new(bytes.Buffer))
				s.ErrorIs(got.err, errInvalidWorkspaceKey)
				s.Empty(got.id)
				s.False(got.quit)
			})
		}
	}

	s.Run("paged, second page picks by the number shown", func() {
		defer testUtil.MockUserInput(s.T(), "6\n")()
		got := getWorkspaceSelection(3, 1, api, new(bytes.Buffer))
		s.NoError(got.err)
		s.Equal(mockWorkspaceList[2].ID, got.id)
	})
}

// A run that may not ask refuses before it fetches or prints the table.
func (s *Suite) TestGetWorkspaceSelectionRefusesWithoutPrintingWhenItMayNotAsk() {
	testUtil.InitTestConfig("software")
	restore := input.SetGuard(func() string { return "with --output json it cannot" })
	defer restore()

	for _, pageSize := range []int{0, 10} {
		api := new(mocks.ClientInterface)
		out := new(bytes.Buffer)
		got := getWorkspaceSelection(pageSize, 0, api, out)
		s.True(input.IsRequired(got.err), "page size %d: %v", pageSize, got.err)
		s.Empty(out.String(), "page size %d", pageSize)
		api.AssertExpectations(s.T())
	}
}
