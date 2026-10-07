package deployment

import (
	"io"
	"strings"

	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	"github.com/astronomer/astro-cli/pkg/input"
)

// --namespace answers both of create's namespace questions, and is checked as
// the answer asked for would be: one the platform offers, or a name that is
// not all spaces. Under a guard that refuses questions, a missing one names
// it.
func (s *Suite) TestNamespaceFromTheFlag() {
	api := new(mocks.ClientInterface)
	api.On("GetAvailableNamespaces", mock.Anything).Return([]houston.Namespace{{Name: "ns-a"}, {Name: "ns-b"}}, nil)
	defer input.SetGuard(func() string { return "with --output json it cannot" })()

	got, err := getDeploymentSelectionNamespaces(api, io.Discard, "c", "ns-b")
	s.NoError(err)
	s.Equal("ns-b", got)

	_, err = getDeploymentSelectionNamespaces(api, io.Discard, "c", "ns-z")
	s.EqualError(err, `namespace "ns-z" is not one this platform offers; use one of: ns-a, ns-b`)

	_, err = getDeploymentSelectionNamespaces(api, io.Discard, "c", "")
	s.ErrorContains(err, "--namespace")

	got, err = getDeploymentNamespaceName("my-ns")
	s.NoError(err)
	s.Equal("my-ns", got)

	_, err = getDeploymentNamespaceName("   ")
	s.ErrorIs(err, ErrKubernetesNamespaceNotSpecified)

	_, err = getDeploymentNamespaceName("")
	s.ErrorContains(err, "--namespace")
}

// A free-form name is checked as Houston checks it: a DNS-1123 label of at
// most 63 characters.
func (s *Suite) TestFreeFormNamespaceFollowsHoustonsRule() {
	for _, ok := range []string{"a", "my-ns", "ns-1", strings.Repeat("a", 63)} {
		got, err := getDeploymentNamespaceName(ok)
		s.NoError(err, ok)
		s.Equal(ok, got)
	}
	got, err := getDeploymentNamespaceName("  padded  ")
	s.NoError(err)
	s.Equal("padded", got, "trimmed, as Houston would refuse the spaces")
	for _, bad := range []string{"Upper", "under_score", "-lead", "trail-", "dot.ted", strings.Repeat("a", 64)} {
		_, err := getDeploymentNamespaceName(bad)
		s.ErrorContains(err, "is not a valid name", bad)
	}
}

// When both namespace settings are on, Houston takes the free-form name and
// skips the pre-created list (houston-api
// ); when neither is, it ignores
// a namespace and names one itself.
func (s *Suite) TestCreateNamespaceSettings() {
	req := &CreateDeploymentRequest{Label: "l", WS: "ws", Executor: houston.CeleryExecutorType, ClusterID: "c", Namespace: "not-in-the-pool", TriggererReplicas: -1}

	both := &houston.AppConfig{Flags: houston.FeatureFlags{ManualNamespaceNames: true, NamespaceFreeFormEntry: true}}
	api := new(mocks.ClientInterface)
	api.On("CreateDeployment", mock.MatchedBy(func(vars map[string]interface{}) bool { return vars["namespace"] == "not-in-the-pool" })).Return(&houston.Deployment{ID: "d"}, nil)
	_, err := Create(req, api, io.Discard, both)
	s.NoError(err, "free-form wins: the pool is not consulted")
	api.AssertNotCalled(s.T(), "GetAvailableNamespaces", mock.Anything)

	neither := &houston.AppConfig{}
	_, err = Create(req, new(mocks.ClientInterface), io.Discard, neither)
	s.ErrorIs(err, errNamespaceNotAsked)
}
