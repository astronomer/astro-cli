//go:build windows

package airflow

import (
	"errors"

	"github.com/astronomer/astro-cli/airflow/types"
	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/pkg/browser"
)

var (
	standaloneOpenURL  = browser.OpenURL
	checkPortAvailable = func(_ string) error { return nil }
	resolveFloatingTag = airflowversions.ResolveFloatingTag
)

// Standalone is a stub on Windows where standalone mode is not supported.
type Standalone struct {
	airflowHome string
	envFile     string
	dockerfile  string
}

var errStandaloneWindows = errors.New("standalone mode is not supported on Windows. Use Docker mode instead: astro config set -g dev.mode docker")

func StandaloneInit(airflowHome, envFile, dockerfile string) (*Standalone, error) {
	return nil, errStandaloneWindows
}

func (s *Standalone) Start(_ *types.StartOptions) error { return errStandaloneWindows }
func (s *Standalone) Stop(_ bool) error                 { return errStandaloneWindows }
func (s *Standalone) PS() error                         { return errStandaloneWindows }
func (s *Standalone) Kill() error                       { return errStandaloneWindows }
func (s *Standalone) Logs(_ bool, _ ...string) error    { return errStandaloneWindows }
func (s *Standalone) Run(_ []string, _ string) error    { return errStandaloneWindows }
func (s *Standalone) Bash(_ string) error               { return errStandaloneWindows }
func (s *Standalone) Build(_, _ string, _ bool) error   { return errStandaloneWindows }
func (s *Standalone) RunDAG(_, _, _, _ string, _, _ bool) error {
	return errStandaloneWindows
}
func (s *Standalone) ImportSettings(_, _ string, _, _, _ bool) error { return errStandaloneWindows }
func (s *Standalone) ExportSettings(_, _ string, _, _, _, _ bool) error {
	return errStandaloneWindows
}
func (s *Standalone) ComposeExport(_, _ string) error { return errStandaloneWindows }
func (s *Standalone) Pytest(_, _, _, _, _ string) (string, error) {
	return "", errStandaloneWindows
}
func (s *Standalone) Parse(_, _, _ string) error { return errStandaloneWindows }
func (s *Standalone) UpgradeTest(_, _, _, _ string, _, _, _, _, _ bool, _ string, _ astroplatformcore.ClientWithResponsesInterface) error {
	return errStandaloneWindows
}
