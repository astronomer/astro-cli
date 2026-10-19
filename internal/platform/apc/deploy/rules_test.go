package deploy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	houston_mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
)

// --image is refused for exactly the Deployments dagsFrom places as image
// ones: where the type was read, one with none too; where it was not (before
// 0.29.0), nothing untyped is refused. An image deployed alone, from outside
// any project, is not refused, and neither is a deploy with DAGs.
func TestRefuseImageOnly(t *testing.T) {
	for _, tc := range []struct {
		typ     string
		read    bool
		refused bool
	}{
		{houston.ImageDeploymentType, true, true},
		{houston.ImageDeploymentType, false, true},
		{"", true, true},
		{"", false, false},
		{houston.DagOnlyDeploymentType, true, false},
		{houston.GitSyncDeploymentType, true, false},
		{houston.VolumeDeploymentType, true, false},
	} {
		info := &houston.Deployment{DagDeployment: houston.DagDeploymentConfig{Type: tc.typ}, DagDeploymentRead: tc.read}
		target := &manifestTarget{dags: dagsFrom(info, &houston.AppConfig{Flags: houston.FeatureFlags{DagOnlyDeployment: true}})}
		err := refuseImageOnly(&ManifestImage{DeploymentID: "dep"}, target)
		if tc.refused {
			assert.ErrorIs(t, err, ErrDeploymentTypeIncorrectForImageOnly, "%q read %v", tc.typ, tc.read)
		} else {
			assert.NoError(t, err, "%q read %v", tc.typ, tc.read)
		}
		assert.NoError(t, refuseImageOnly(&ManifestImage{DeploymentID: "dep", ImageAlone: true}, target), "alone: %q read %v", tc.typ, tc.read)
		assert.NoError(t, refuseImageOnly(&ManifestImage{DeploymentID: "dep", IncludeDags: true}, target), "with DAGs: %q read %v", tc.typ, tc.read)
	}
}

// A deploy's Deployment is settled before the Deployment is read, and held
// to the workspace the deploy runs in: no workspace, no Deployment, one the
// workspace does not hold and a Houston that cannot answer are refused.
func TestWorkspaceDeployments(t *testing.T) {
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0"}).client(t)
	_, err := workspaceDeployments(client, "", "dep")
	assert.ErrorIs(t, err, ErrNoWorkspaceID)
	_, err = workspaceDeployments(client, "ws", "")
	assert.ErrorIs(t, err, errInvalidDeploymentID)
	_, err = workspaceDeployments(client, "ws", "elsewhere")
	assert.ErrorIs(t, err, errInvalidDeploymentID)
	deps, err := workspaceDeployments(client, "ws", "dep")
	require.NoError(t, err)
	assert.Len(t, deps, 1)

	failing := new(houston_mocks.ClientInterface)
	failing.On("GetWorkspace", "ws").Return(nil, errMockHouston).Once()
	_, err = workspaceDeployments(failing, "ws", "dep")
	assert.ErrorIs(t, err, errMockHouston)
	failing.On("GetWorkspace", "ws").Return(&houston.Workspace{ID: "ws"}, nil)
	failing.On("ListDeployments", mock.Anything).Return(nil, errMockHouston).Once()
	_, err = workspaceDeployments(failing, "ws", "dep")
	assert.ErrorIs(t, err, errMockHouston)
}

// From outside any project a name is read as a Deployment id, as there are no
// links: one the workspace does not hold, in no shape a Houston id has, says
// a link name resolves only from the project's directory. An id-shaped one,
// and any from a project, is told only that the workspace does not hold it.
func TestALinkNameOutsideAProjectIsHinted(t *testing.T) {
	const hint = "outside a pyproject.toml project there are no links, so a link name resolves only when the deploy runs from the project's directory"
	for _, tc := range []struct {
		name  string
		id    string
		alone bool
		hint  bool
	}{
		{"a link name outside a project", "prod", true, true},
		{"an id outside a project", "ckn4phn1k0104v5xtrer5lpli", true, false},
		{"a link name from a project", "prod", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, remote := range []bool{false, true} {
				withManifestEngine(t, "3.1-5")
				client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0"}).client(t)
				var warnings []string
				in := prebuiltFor(t, "my-image:v7", &warnings)
				if remote {
					in.InRegistry, in.RuntimeVersion = true, "3.1-5"
				}
				in.DeploymentID = tc.id
				if tc.alone {
					in.IncludeDags, in.ImageAlone = false, true
				}
				_, err := DeployManifestImage(client, in)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "no Deployment "+tc.id+" in workspace ws", "remote=%v", remote)
				if tc.hint {
					assert.Contains(t, err.Error(), hint, "remote=%v", remote)
				} else {
					assert.NotContains(t, err.Error(), hint, "remote=%v", remote)
				}
			}
		})
	}
}
