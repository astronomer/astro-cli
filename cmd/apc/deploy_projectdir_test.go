package apc

import (
	"os"
	"path/filepath"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/deploy"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
)

// The project check follows the deploy asked for. A DAG-only deploy builds
// nothing, so a 1.x project without a Dockerfile and a pyproject.toml project
// both make one; an image deploy needs the .astro/config.yaml and the
// Dockerfile, and says which is missing. A non-empty --image-name skips the
// check; an empty one does not.
func (s *Suite) TestDeployProjectCheckFollowsTheDeploy() {
	appConfig = &houston.AppConfig{}
	prevEnsure, prevPath, prevImage := EnsureProjectDir, config.WorkingPath, DeployAirflowImage
	defer func() { EnsureProjectDir, config.WorkingPath, DeployAirflowImage = prevEnsure, prevPath, prevImage }()
	EnsureProjectDir = ensureDeployProjectDir
	DeployAirflowImage = func(houston.ClientInterface, string, string, string, bool, bool, string, bool, string, deploy.Options) (deploy.Deployed, error) {
		return deploy.Deployed{DeploymentID: "dep"}, nil
	}
	DagsOnlyDeploy = func(_ houston.ClientInterface, _, deploymentID, _ string, _ *string, _ bool, _ string, _ deploy.Options) (string, error) {
		return deploymentID, nil
	}

	write := func(dir, name, content string) {
		s.Require().NoError(os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755))
		s.Require().NoError(os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
	pyproject := s.T().TempDir()
	write(pyproject, "pyproject.toml", "[project]\nname = \"demo\"\n\n[tool.astro]\n")
	noDockerfile := s.T().TempDir()
	write(noDockerfile, filepath.Join(".astro", "config.yaml"), "project:\n  name: demo\n")
	empty := s.T().TempDir()

	config.WorkingPath = pyproject
	s.NoError(execDeployCmd("dep", "--dags", "--force"), "a pyproject.toml project makes a DAG-only deploy")
	err := execDeployCmd("dep", "--force")
	s.ErrorContains(err, "has no .astro/config.yaml and no Dockerfile")
	s.ErrorContains(err, "astro package --tag <image>, then astro deploy <deployment-id> --image-name <image>")
	s.NoError(execDeployCmd("dep", "--force", "--image-name", "img:1"), "--image-name needs no project")

	config.WorkingPath = noDockerfile
	s.NoError(execDeployCmd("dep", "--dags", "--force"), "a DAG-only deploy needs no Dockerfile")
	err = execDeployCmd("dep", "--force")
	s.ErrorContains(err, "has no Dockerfile")
	s.NotContains(err.Error(), "no .astro/config.yaml")

	config.WorkingPath = empty
	s.ErrorContains(execDeployCmd("dep", "--dags", "--force"), "run astro init")
	s.ErrorContains(execDeployCmd("dep", "--force", "--image-name="), "has no .astro/config.yaml and no Dockerfile", "an empty --image-name= still checks the project")
}
