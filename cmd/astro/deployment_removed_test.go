package astro

import (
	"testing"

	"github.com/stretchr/testify/assert"

	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// The groups that wrote Airflow objects through the Airflow REST API are
// tombstones: every old spelling must fail, name the `astro env` replacement,
// and reach that guidance with its old flags intact and without an API call.
func TestRemovedDeploymentObjectGroupsAreTombstones(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"connection list", []string{"connection", "list", "--deployment-id", "d"}, "astro env connection list --deployment <deployment-id>"},
		{"con alias", []string{"con", "li", "-d", "d"}, "astro env connection list --deployment <deployment-id>"},
		{"connection create", []string{"connections", "create", "--conn-id", "c", "--conn-type", "http"}, "astro env connection set <key>"},
		{"connection update", []string{"connection", "up", "--conn-id", "c"}, "astro env connection set <key>"},
		{"connection delete", []string{"connection", "rm", "--conn-id", "c", "-f"}, "astro env connection delete <key>"},
		{"connection copy", []string{"connection", "cp", "--source-id", "a", "--target-id", "b"}, "--auto-link"},
		{"airflow-variable create", []string{"airflow-variable", "create", "--key", "k", "--value", "v"}, "astro env airflow-variable set <key>"},
		{"airflow-var alias", []string{"airflow-var", "list"}, "astro env airflow-variable list"},
		{"bare group", []string{"airflow-variables"}, "astro env airflow-variable --help"},
		{"pool", []string{"pool", "create", "--name", "p", "--slots", "3"}, "Airflow UI"},
		{"pl alias", []string{"pl", "list"}, "not in the Environment Manager yet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			astroV1Client = mc

			_, err := execDeploymentCmd(tc.args...)
			assert.Error(t, err, "a removed command must fail, not print help and exit 0")
			assert.Contains(t, err.Error(), "was removed in v2")
			assert.Contains(t, err.Error(), tc.want)
			mc.AssertExpectations(t)
		})
	}
}

// Help lists only what exists.
func TestRemovedDeploymentObjectGroupsAreHidden(t *testing.T) {
	for _, c := range newRemovedDeploymentObjectCmds() {
		assert.True(t, c.Hidden, c.Name())
	}
}
