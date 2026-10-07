package deployment

import (
	"errors"

	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
)

// A runtime cancel on each Houston that serves runtime upgrades:
//   - 1.0.0–1.0.42 select desiredRuntimeVersion (houston DeploymentGetRequest
//     1.0.0/1.0.1), so a pending upgrade is canceled, and with none pending
//     cancelRuntimeUpdate is not called at all; Houston would refuse it with
//     CancelRuntimeUpdateError.
//   - 1.0.43 and later removed cancelRuntimeUpdate and the desired version
//    . The call is gated off (ErrAPINotImplemented)
//     and there is nothing pending to cancel.
func (s *Suite) TestRuntimeCancelOnEachHouston() {
	s.Run("1.0.x, an upgrade pending", func() {
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", "rt").Return(&houston.Deployment{ID: "rt", RuntimeVersion: "9.1.0", DesiredRuntimeVersion: "9.2.0"}, nil)
		api.On("CancelUpdateDeploymentRuntime", mock.Anything).Return(&houston.Deployment{ID: "rt"}, nil)

		got, err := RuntimeUpgradeCancel("rt", api)
		s.NoError(err)
		s.Equal(VersionChangeCanceled, got.Action)
	})

	s.Run("1.0.x, nothing pending", func() {
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", "rt").Return(&houston.Deployment{ID: "rt", RuntimeVersion: "9.1.0", DesiredRuntimeVersion: "9.1.0"}, nil)

		got, err := RuntimeUpgradeCancel("rt", api)
		s.NoError(err)
		s.Equal(VersionChangeNothingToCancel, got.Action)
		api.AssertNotCalled(s.T(), "CancelUpdateDeploymentRuntime", mock.Anything)
	})

	s.Run("1.0.43 and later", func() {
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", "rt").Return(&houston.Deployment{ID: "rt", RuntimeVersion: "9.1.0"}, nil)
		api.On("CancelUpdateDeploymentRuntime", mock.Anything).Return(nil, houston.ErrAPINotImplemented{APIName: "CancelUpdateDeploymentRuntime"})

		got, err := RuntimeUpgradeCancel("rt", api)
		s.NoError(err)
		s.Equal(VersionChangeNothingToCancel, got.Action)
		s.Equal(ImageVersion{Image: ImageRuntime, Version: "9.1.0"}, got.Current)
	})

	s.Run("any other failure is one", func() {
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", "rt").Return(&houston.Deployment{ID: "rt", RuntimeVersion: "9.1.0", DesiredRuntimeVersion: "9.2.0"}, nil)
		api.On("CancelUpdateDeploymentRuntime", mock.Anything).Return(nil, errors.New("Insufficient permissions."))

		_, err := RuntimeUpgradeCancel("rt", api)
		s.EqualError(err, "Insufficient permissions.")
	})
}

// A cancel reports the image the Deployment runs, whichever command it came
// from. They used to read the field their own command is about, which on the
// other kind of Deployment is empty: "runtime", "" for an Astronomer
// Certified Deployment.
func (s *Suite) TestCancelsReportTheImageTheDeploymentRuns() {
	certified := &houston.Deployment{ID: "ac", AirflowVersion: "2.2.4", DesiredAirflowVersion: "2.2.4"}
	onRuntime := &houston.Deployment{ID: "rt", RuntimeVersion: "4.2.0", DesiredRuntimeVersion: "4.2.0"}
	migrating := &houston.Deployment{ID: "mig", AirflowVersion: "2.2.4", DesiredRuntimeVersion: "4.2.0"}

	api := new(mocks.ClientInterface)
	api.On("GetDeployment", "ac").Return(certified, nil)
	api.On("GetDeployment", "rt").Return(onRuntime, nil)
	api.On("GetDeployment", "mig").Return(migrating, nil)
	api.On("CancelUpdateDeploymentRuntime", mock.Anything).Return(migrating, nil)

	runtime := ImageVersion{Image: ImageRuntime, Version: "4.2.0"}
	ac := ImageVersion{Image: ImageCertified, Version: "2.2.4"}

	got, err := AirflowUpgradeCancel("rt", api)
	s.NoError(err)
	s.Equal(VersionChangeNothingToCancel, got.Action)
	s.Equal(runtime, got.Current, "an Airflow upgrade cancel on a Runtime Deployment")

	got, err = RuntimeMigrateCancel("ac", api)
	s.NoError(err)
	s.Equal(VersionChangeNothingToCancel, got.Action)
	s.Equal(ac, got.Current, "a migrate cancel on a Deployment that never started one")

	got, err = RuntimeUpgradeCancel("mig", api)
	s.NoError(err)
	s.Equal(VersionChangeCanceled, got.Action)
	s.Equal(ac, got.Current, "a Runtime cancel on a Deployment still on Astronomer Certified")
}
