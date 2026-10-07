package workspace

import (
	"bytes"
	"io"
	"os"
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
		Description:  new("description"),
		Users:        nil,
		CreatedAt:    "",
		UpdatedAt:    "",
		RoleBindings: nil,
	}
	mockWorkspaceList = []houston.Workspace{
		{
			ID:           "ckbv7zvb100pe0760xp98qnh9",
			Label:        "w1",
			Description:  new("description"),
			Users:        nil,
			CreatedAt:    "",
			UpdatedAt:    "",
			RoleBindings: nil,
		},
		{
			ID:           "ckbv8pwbq00wk0760us7ktcgd",
			Label:        "wwww",
			Description:  new("description"),
			Users:        nil,
			CreatedAt:    "",
			UpdatedAt:    "",
			RoleBindings: nil,
		},
		{
			ID:           "ckc0j8y1101xo0760or02jdi7",
			Label:        "test",
			Description:  new("description"),
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

	w, err := Create(label, description, api)
	s.NoError(err)
	s.Equal(mockWorkspace, w)
	api.AssertExpectations(s.T())
}

// Houston answers createWorkspace and updateWorkspace with the record it
// wrote or an error, never null (houston-api
// 
// update-workspace/). A null with no error is refused rather
// than printed from nothing, which used to panic.
func (s *Suite) TestCreateAndUpdateRefuseNoWorkspace() {
	testUtil.InitTestConfig("software")
	api := new(mocks.ClientInterface)
	api.On("CreateWorkspace", houston.CreateWorkspaceRequest{Label: "l", Description: "d"}).Return(nil, nil)
	api.On("UpdateWorkspace", houston.UpdateWorkspaceRequest{WorkspaceID: "id", Args: map[string]string{"label": "l"}}).Return(nil, nil)

	w, err := Create("l", "d", api)
	s.ErrorIs(err, errNoWorkspace)
	s.Nil(w)
	w, err = Update("id", api, map[string]string{"label": "l"})
	s.ErrorIs(err, errNoWorkspace)
	s.Nil(w)
	api.AssertExpectations(s.T())
}

func (s *Suite) TestCreateError() {
	testUtil.InitTestConfig("software")

	label := "test"
	description := "description"

	api := new(mocks.ClientInterface)
	api.On("CreateWorkspace", houston.CreateWorkspaceRequest{Label: label, Description: description}).Return(nil, errMock)

	_, err := Create(label, description, api)
	s.EqualError(err, errMock.Error())
	api.AssertExpectations(s.T())
}

func (s *Suite) TestList() {
	testUtil.InitTestConfig("software")

	api := new(mocks.ClientInterface)
	api.On("ListWorkspaces", nil).Return(mockWorkspaceList, nil)

	ws, err := List(api)
	s.NoError(err)
	s.Equal(mockWorkspaceList, ws)
	api.AssertExpectations(s.T())
}

func (s *Suite) TestListError() {
	testUtil.InitTestConfig("software")

	api := new(mocks.ClientInterface)
	api.On("ListWorkspaces", nil).Return(nil, errMock)

	_, err := List(api)
	s.EqualError(err, errMock.Error())
	api.AssertExpectations(s.T())
}

// Houston answers deleteWorkspace with the record it removed: id, label and
// description, which is what the CLI's mutation asks for (houston-api
// ).
func (s *Suite) TestDelete() {
	testUtil.InitTestConfig("software")

	mockResponse := &houston.Workspace{
		ID:          "ckc0j8y1101xo0760or02jdi7",
		Label:       "test",
		Description: new("description"),
	}

	api := new(mocks.ClientInterface)
	api.On("DeleteWorkspace", mockResponse.ID).Return(mockResponse, nil)

	w, err := Delete(mockResponse.ID, api)
	s.NoError(err)
	s.Equal(mockResponse, w)
	api.AssertExpectations(s.T())
}

func (s *Suite) TestDeleteError() {
	testUtil.InitTestConfig("software")

	wsID := "ckc0j8y1101xo0760or02jdi7"

	api := new(mocks.ClientInterface)
	api.On("DeleteWorkspace", wsID).Return(nil, errMock)

	_, err := Delete(wsID, api)
	s.EqualError(err, errMock.Error())
	api.AssertExpectations(s.T())
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
	sel := getWorkspaceSelection(0, 0, "", api, buf)
	s.EqualError(sel.err, errMock.Error())
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

	w, quit, err := Switch("", 0, "", api)
	s.NoError(err)
	s.False(quit)
	s.Equal(mockWorkspace, w)
	c, err := config.GetCurrentContext()
	s.NoError(err)
	s.Equal(mockWorkspace.ID, c.Workspace, "the workspace picked is the context's now")
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

	var (
		w    *houston.Workspace
		quit bool
		err  error
	)
	asked := captureStderr(s.T(), func() { w, quit, err = Switch("", 10, "", api) })
	s.NoError(err)
	s.True(quit, "a quit is told apart from a switch")
	// The paged table and its prompt are the question, both on stderr.
	s.Contains(asked, mockWorkspace.ID)
	s.Contains(asked, "Please select one of the following options")
	s.Nil(w, "quitting switches to nothing")
	c, err := config.GetCurrentContext()
	s.NoError(err)
	s.Empty(c.Workspace, "quitting leaves the context as it was")
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

	var err error
	asked := captureStderr(s.T(), func() { _, _, err = Switch("", 0, "", api) })
	s.ErrorIs(err, errInvalidWorkspaceKey)
	// The picker is the question: on stderr.
	s.Contains(asked, mockWorkspace.ID)
	api.AssertExpectations(s.T())
}

// captureStderr returns what f writes to the process's stderr, which it swaps
// for a pipe while f runs.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stderr
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	f()
	os.Stderr = previous
	w.Close()
	return <-done
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

	_, _, err := Switch(wsID, 0, "", api)
	s.EqualError(err, "workspace id is not valid: api error")
	api.AssertExpectations(s.T())
}

// The client turns Houston's null into ErrWorkspaceNotFound, so a nil
// workspace with no error is not one to switch to: it is an error, not a
// quit, and the context is left as it was.
func (s *Suite) TestSwitchRefusesNoWorkspace() {
	testUtil.InitTestConfig("software")
	before, err := config.GetCurrentContext()
	s.Require().NoError(err)

	api := new(mocks.ClientInterface)
	api.On("ValidateWorkspaceID", "ckbv7zvb100pe0760xp98qnh9").Return(nil, nil)

	w, quit, err := Switch("ckbv7zvb100pe0760xp98qnh9", 0, "", api)
	s.ErrorIs(err, errNoWorkspace)
	s.False(quit)
	s.Nil(w)
	after, err := config.GetCurrentContext()
	s.Require().NoError(err)
	s.Equal(before.Workspace, after.Workspace)
	api.AssertExpectations(s.T())
}

// Pages go to Houston in its own numbering for the version given: the second
// page of three is 2 on a Houston from v0.31.6 on and 1 before it, and the
// first is 0 on both.
func (s *Suite) TestGetWorkspaceSelectionNumbersPagesForTheVersionGiven() {
	testUtil.InitTestConfig("software")
	for _, tc := range []struct {
		version          string
		first, secondOut int
	}{{"1.0.0", 0, 2}, {"0.31.6", 0, 2}, {"0.31.5", 0, 1}, {"0.30.3", 0, 1}} {
		s.Run(tc.version, func() {
			api := new(mocks.ClientInterface)
			api.On("PaginatedListWorkspaces", houston.PaginatedListWorkspaceRequest{PageSize: 3, PageNumber: tc.first}).Return(mockWorkspaceList, nil).Twice()
			api.On("PaginatedListWorkspaces", houston.PaginatedListWorkspaceRequest{PageSize: 3, PageNumber: tc.secondOut}).Return(mockWorkspaceList[:1], nil).Once()
			defer testUtil.MockUserInput(s.T(), "n\nf\n1\n")()
			got := getWorkspaceSelection(3, 0, tc.version, api, new(bytes.Buffer))
			s.NoError(got.err)
			s.Equal(mockWorkspaceList[0].ID, got.id)
			api.AssertExpectations(s.T())
		})
	}
}

func (s *Suite) TestUpdate() {
	testUtil.InitTestConfig("software")

	id := "test"
	args := map[string]string{"1": "2"}

	api := new(mocks.ClientInterface)
	api.On("UpdateWorkspace", houston.UpdateWorkspaceRequest{WorkspaceID: id, Args: args}).Return(mockWorkspace, nil)

	w, err := Update(id, api, args)
	s.NoError(err)
	s.Equal(mockWorkspace, w)
	api.AssertExpectations(s.T())
}

func (s *Suite) TestUpdateError() {
	testUtil.InitTestConfig("software")

	// prepare houston-api fake response
	id := "test"
	args := map[string]string{"1": "2"}

	api := new(mocks.ClientInterface)
	api.On("UpdateWorkspace", houston.UpdateWorkspaceRequest{WorkspaceID: id, Args: args}).Return(nil, errMock)

	_, err := Update(id, api, args)
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
		sel := getWorkspaceSelection(0, 0, "", api, out)

		s.Contains(sel.err.Error(), "no context set, have you authenticated to Astro or APC? Run astro login and try again")
		s.Equal("", sel.id)
	})

	testUtil.InitTestConfig("software")

	s.Run("success", func() {
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(s.T(), "1")()
		sel := getWorkspaceSelection(0, 0, "", api, out)

		s.NoError(sel.err)
		s.Equal("ckbv7zvb100pe0760xp98qnh9", sel.id)
	})

	s.Run("success with pagination", func() {
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(s.T(), "1")()
		sel := getWorkspaceSelection(10, 0, "", api, out)

		s.NoError(sel.err)
		s.Equal("ckbv7zvb100pe0760xp98qnh9", sel.id)
	})

	s.Run("invalid selection", func() {
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(s.T(), "y")()
		sel := getWorkspaceSelection(0, 0, "", api, out)

		s.ErrorIs(sel.err, errInvalidWorkspaceKey)
		s.Equal("", sel.id)
	})

	s.Run("quit selection when paginated", func() {
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(s.T(), "q")()
		sel := getWorkspaceSelection(10, 0, "", api, out)
		s.Nil(sel.err)
		s.Equal("", sel.id)
		s.Equal(true, sel.quit)
	})
}

// The paged switch shows a page of the list, numbered on from the pages
// before, and the letters on offer for it; a wrong answer is told so and
// asked again, with the letters, up to picker.DefaultAttempts answers, and a
// letter or a row number among them still answers. Input that ends is not
// asked again.
func (s *Suite) TestGetWorkspaceSelectionPaged() {
	testUtil.InitTestConfig("software")
	const told = "Not one of the choices."
	// Pages of three: a full first page offers "n"; the second page offers
	// "f" and "p" and, full too, "n".
	api := new(mocks.ClientInterface)
	api.On("PaginatedListWorkspaces", houston.PaginatedListWorkspaceRequest{PageSize: 3, PageNumber: 0}).Return(mockWorkspaceList, nil)
	api.On("PaginatedListWorkspaces", houston.PaginatedListWorkspaceRequest{PageSize: 3, PageNumber: 2}).Return(mockWorkspaceList, nil)
	api.On("PaginatedListWorkspaces", houston.PaginatedListWorkspaceRequest{PageSize: 10, PageNumber: 0}).Return(mockWorkspaceList, nil)
	ask := func(pageSize int, in string) (workspaceSelection, string) {
		defer testUtil.MockUserInput(s.T(), in)()
		out := new(bytes.Buffer)
		return getWorkspaceSelection(pageSize, 0, "", api, out), out.String()
	}

	s.Run("a row after two typos", func() {
		got, out := ask(3, "x\n0\n2\n")
		s.NoError(got.err)
		s.Equal(mockWorkspaceList[1].ID, got.id)
		s.Equal(2, strings.Count(out, told+"\nPlease select one of the following options or enter index to select the row.\nn. next q. quit\n> "), out)
	})

	s.Run("a page letter after a typo, then a row of the next page", func() {
		got, out := ask(3, "nn\nn\n6\n")
		s.NoError(got.err)
		s.Equal(mockWorkspaceList[2].ID, got.id, "row 6 is the third of the second page")
		s.Equal(1, strings.Count(out, told), out)
		s.Contains(out, "f. first p. previous n. next q. quit\n> ")
	})

	s.Run("quit", func() {
		got, _ := ask(3, "q\n")
		s.NoError(got.err)
		s.True(got.quit)
	})

	s.Run("a page not full offers no next", func() {
		got, out := ask(10, "n\n")
		s.ErrorIs(got.err, errInvalidWorkspaceKey)
		s.Contains(out, "\nq. quit\n> ")
		s.NotContains(out, "n. next")
	})

	s.Run("three wrong answers", func() {
		got, out := ask(3, "x\ny\nz\n1\n")
		s.ErrorIs(got.err, errInvalidWorkspaceKey)
		s.Equal(3, strings.Count(out, told), out)
		s.True(strings.HasSuffix(out, told+"\n"), out)
	})

	s.Run("closed stdin ends at once", func() {
		got, out := ask(3, "")
		s.ErrorIs(got.err, errInvalidWorkspaceKey)
		s.NotContains(out, told)
	})

	s.Run("an answer cut short by the end of input", func() {
		got, out := ask(3, "x")
		s.ErrorIs(got.err, errInvalidWorkspaceKey)
		s.Equal(1, strings.Count(out, told), out)
	})
}

// The switch asks again after a typo, paged or not, so a slip does not
// abandon it (nor the `astro login` that asks it).
func (s *Suite) TestGetWorkspaceSelectionAsksAgain() {
	testUtil.InitTestConfig("software")
	api := new(mocks.ClientInterface)
	api.On("ListWorkspaces", nil).Return(mockWorkspaceList, nil)
	api.On("PaginatedListWorkspaces", houston.PaginatedListWorkspaceRequest{PageSize: 10, PageNumber: 0}).Return(mockWorkspaceList, nil)

	for _, pageSize := range []int{0, 10} {
		s.Run(map[int]string{0: "not paged", 10: "paged"}[pageSize], func() {
			defer testUtil.MockUserInput(s.T(), "w1\n2\n")()
			out := new(bytes.Buffer)
			got := getWorkspaceSelection(pageSize, 0, "", api, out)
			s.NoError(got.err)
			s.Equal(mockWorkspaceList[1].ID, got.id)
			s.Contains(out.String(), "Not one of the choices.")
		})
	}
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
	api.On("PaginatedListWorkspaces", houston.PaginatedListWorkspaceRequest{PageSize: 3, PageNumber: 2}).Return(mockWorkspaceList, nil)

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
				got := getWorkspaceSelection(tc.pageSize, tc.pageNumber, "", api, new(bytes.Buffer))
				s.ErrorIs(got.err, errInvalidWorkspaceKey)
				s.Empty(got.id)
				s.False(got.quit)
			})
		}
	}

	s.Run("paged, second page picks by the number shown", func() {
		defer testUtil.MockUserInput(s.T(), "6\n")()
		got := getWorkspaceSelection(3, 1, "", api, new(bytes.Buffer))
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
		got := getWorkspaceSelection(pageSize, 0, "", api, out)
		s.True(input.IsRequired(got.err), "page size %d: %v", pageSize, got.err)
		s.Empty(out.String(), "page size %d", pageSize)
		api.AssertExpectations(s.T())
		// The early refusal is the question's own: the one asking it gives.
		list := switchList(pageSize > 0)
		s.Equal(list.MayAsk().Error(), got.err.Error(), "page size %d", pageSize)
		s.Equal(pageSize > 0, strings.Contains(got.err.Error(), "which page to show next"), "page size %d: %v", pageSize, got.err)
	}
}
