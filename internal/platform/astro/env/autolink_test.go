package env

import (
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
)

// Auto-link is a workspace-scope concept. `set` tries the update before the
// create, so a guard on the create path alone let an existing deployment-scoped
// object reach the API with autoLinkDeployments=true. Every update refuses it
// before any call; the mock has no expectations, so a request would fail it.
func (s *Suite) TestUpdatesRefuseAutoLinkAtDeploymentScope() {
	scope := Scope{DeploymentID: "dep"}
	on := true
	for name, update := range map[string]func(astrov1.APIClient) error{
		"variable": func(c astrov1.APIClient) error {
			_, err := UpdateVar("FOO", scope, "v", &on, c)
			return err
		},
		"airflow-variable": func(c astrov1.APIClient) error {
			_, err := UpdateAirflowVar("foo", scope, "v", &on, c)
			return err
		},
		"connection": func(c astrov1.APIClient) error {
			_, err := UpdateConn("foo", scope, ConnInput{Type: "http", AutoLinkDeployments: &on}, c)
			return err
		},
		"metrics-export": func(c astrov1.APIClient) error {
			_, err := UpdateMetricsExport("foo", scope, &MetricsInput{AutoLinkDeployments: &on}, c)
			return err
		},
	} {
		s.Run(name, func() {
			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			s.ErrorIs(update(mc), ErrAutoLinkRequiresWorkspace)
			mc.AssertExpectations(s.T())
		})
	}
}
