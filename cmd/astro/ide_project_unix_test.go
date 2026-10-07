//go:build !windows

package astro

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
)

// An export archives the directory before it asks or creates anything, so a
// local failure (here a file it cannot read) creates no project in the Astro
// IDE. Unix only: Windows has no unreadable mode to set.
func TestIDEProjectExportFailingLocallyCreatesNoProject(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads any file")
	}
	etl, ml := ideProjects()
	fresh := astrov1alpha1.AstroIdeProject{Id: "proj-new", Name: "Fresh"}
	tc := ideCase{projects: []astrov1alpha1.AstroIdeProject{etl, ml}, files: map[string]string{"dags/a.py": "x=1\n"}, answers: "y\nFresh\n", args: []string{"project", "export"}}
	dir := ideDir(t, tc.files)
	require.NoError(t, os.Chmod(filepath.Join(dir, "dags", "a.py"), 0o000))
	m := ideMock(t, tc.projects, opensSession(astrov1alpha1.CreateAstroIdeSessionPermissionREADWRITE), acceptsUpload, createsProject(fresh))
	r := execIDECmd(t, m, tc.answers, tc.args...)
	require.Error(t, r.err)
	assert.Equal(t, cliout.ExitFailure, r.code)
	assert.Contains(t, r.err.Error(), "permission denied")
	assert.NotContains(t, r.stdout, "create a new project")
	m.AssertNotCalled(t, "CreateAstroIdeProjectWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
