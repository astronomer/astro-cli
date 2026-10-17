package cmd

import (
	"bytes"
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/local"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	houston_mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// scriptAPCPick answers the APC Deployment picker with answer and captures
// what it drew.
func scriptAPCPick(t *testing.T, answer string) *bytes.Buffer {
	t.Helper()
	origIn, origOut := apcLinkPickIn, apcLinkPickOut
	t.Cleanup(func() { apcLinkPickIn, apcLinkPickOut = origIn, origOut })
	var drawn bytes.Buffer
	apcLinkPickIn, apcLinkPickOut = strings.NewReader(answer), &drawn
	return &drawn
}

// Under an APC context the root wires astro link add's APC side: the
// context's domain and workspace, and a picker over Houston's Deployments.
// The Astro pickers stay nil.
func TestAPCLinkingIsWiredUnderAPC(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	var d local.Deps
	wireLinkPickers(&d, apcPlatform, astrov1_mocks.NewClientWithResponsesInterface(t), houston_mocks.NewClientInterface(t), &bytes.Buffer{})
	require.NotNil(t, d.APC)
	assert.Equal(t, "astronomer_dev.com", d.APC.Domain)
	ws, err := d.APC.CurrentWorkspace()
	require.NoError(t, err)
	assert.Equal(t, "ck05r3bor07h40d02y2hw4n4v", ws)
	assert.Nil(t, d.PickDeployment)

	var cloud local.Deps
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	wireLinkPickers(&cloud, cloudPlatform, astrov1_mocks.NewClientWithResponsesInterface(t), nil, &bytes.Buffer{})
	assert.Nil(t, cloud.APC)
}

// withAPCLinking runs as the root is built, for every command under an APC
// context, so it adds no login read there: outside the closures it hands
// astro link add, it reads the current domain and nothing that resolves a
// login (GetCurrentContext and GetContext can reach the OS keyring or the
// secrets vault). The closures read the workspace when link add asks. It
// says nothing of the rest of the root's construction.
func TestAPCLinkingAddsNoLoginRead(t *testing.T) {
	file, err := parser.ParseFile(gotoken.NewFileSet(), "link_pickers_apc.go", nil, 0)
	require.NoError(t, err)
	var body *ast.BlockStmt
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "withAPCLinking" {
			body = fn.Body
		}
	}
	require.NotNil(t, body, "withAPCLinking not found")
	ast.Inspect(body, func(n ast.Node) bool {
		if _, lazy := n.(*ast.FuncLit); lazy {
			return false
		}
		if sel, ok := n.(*ast.SelectorExpr); ok {
			switch sel.Sel.Name {
			case "GetCurrentContext", "GetContext", "ListContexts", "IsCloudContext", "GetCurrentWorkspace":
				t.Errorf("withAPCLinking calls %s as the root is built, which reads a login", sel.Sel.Name)
			}
		}
		return true
	})
}

// The picker lists the workspace's Deployments, leaving out the linked ones,
// and returns the one chosen with its workspace.
func TestAPCLinkPickerListsHoustonDeployments(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	client := houston_mocks.NewClientInterface(t)
	client.On("ListDeployments", mock.MatchedBy(func(r houston.ListDeploymentsRequest) bool { return r.WorkspaceID == "ckws-1" })).
		Return([]houston.Deployment{
			{ID: "ckdep-linked", Label: "Old", ReleaseName: "old-1", Workspace: houston.Workspace{ID: "ckws-1"}},
			{ID: "ckdep-new", Label: "Orders Prod", ReleaseName: "orbit-2", Workspace: houston.Workspace{ID: "ckws-1"}},
		}, nil)
	drawn := scriptAPCPick(t, "1\n")
	var d local.Deps
	withAPCLinking(&d, client)
	dep, err := d.APC.PickDeployment("ckws-1", map[string]string{"ckdep-linked": "old"})
	require.NoError(t, err)
	assert.Equal(t, local.PickedDeployment{ID: "ckdep-new", Name: "Orders Prod", WorkspaceID: "ckws-1"}, dep)
	assert.Contains(t, drawn.String(), "Select a Deployment to link")
	assert.NotContains(t, drawn.String(), "ckdep-linked")
}

// Every Deployment already linked is ErrAllDeploymentsLinked, which add turns
// into the names of the links; an empty workspace says so.
func TestAPCLinkPickerWithNothingToOffer(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	client := houston_mocks.NewClientInterface(t)
	client.On("ListDeployments", mock.Anything).Return([]houston.Deployment{{ID: "ckdep-linked"}}, nil).Once()
	client.On("ListDeployments", mock.Anything).Return([]houston.Deployment{}, nil).Once()
	scriptAPCPick(t, "")
	var d local.Deps
	withAPCLinking(&d, client)
	_, err := d.APC.PickDeployment("ckws-1", map[string]string{"ckdep-linked": "old"})
	require.ErrorIs(t, err, local.ErrAllDeploymentsLinked)
	_, err = d.APC.PickDeployment("ckws-1", nil)
	require.EqualError(t, err, "no Deployments in workspace ckws-1 to link")
}
