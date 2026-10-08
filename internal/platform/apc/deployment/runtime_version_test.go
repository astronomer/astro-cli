package deployment

import (
	"bytes"
	"os"

	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
)

// Runtime versions compare as Houston compares them: an Airflow 3 Runtime,
// M.m-p, is (M*1000).m.p, so it is newer than every Airflow 2 Runtime. Read
// as semver, 3.0-1 was a prerelease of 3.0.0, older than 12.x.
func (s *Suite) TestRuntimeVersionsCompareAsHoustonComparesThem() {
	s.Equal("3000.0.1", normalizeRuntimeVersion("3.0-1"))
	s.Equal("3000.1.12", normalizeRuntimeVersion("3.1-12"))
	s.Equal("12.1.1", normalizeRuntimeVersion("12.1.1"))
	s.Equal("not-a-version", normalizeRuntimeVersion("not-a-version"))

	s.NoError(meetsRuntimeUpgradeReqs("12.1.1", "3.0-1"))
	s.EqualError(meetsRuntimeUpgradeReqs("3.0-1", "3.0-1"),
		"Error: You tried to set --desired-runtime-version to 3.0-1, but this Runtime Deployment is already running 3.0-1. Please indicate a higher version of Runtime and try again.")
	// The message names each as given: what was asked for, and what the
	// Deployment runs, as v2 did.
	s.EqualError(meetsRuntimeUpgradeReqs("4.2.4", "v4.2.4"),
		"Error: You tried to set --desired-runtime-version to v4.2.4, but this Runtime Deployment is already running 4.2.4. Please indicate a higher version of Runtime and try again.")

	// The picker offers a 3.x Runtime to a Deployment on 12.x. Houston 1.0.x
	// returns the newest Runtime per Airflow line.
	api := new(mocks.ClientInterface)
	api.On("GetRuntimeReleases", mock.Anything).Return(houston.RuntimeReleases{
		{Version: "12.1.1", AirflowVersion: "2.10.5"},
		{Version: "3.0-1", AirflowVersion: "3.0.1"},
	}, nil)
	r, w, err := os.Pipe()
	s.Require().NoError(err)
	_, err = w.WriteString("2\n")
	s.Require().NoError(err)
	s.Require().NoError(w.Close())
	stdin := os.Stdin
	defer func() { os.Stdin = stdin }()
	os.Stdin = r

	drawn := new(bytes.Buffer)
	got, err := getRuntimeVersionSelection("12.1.0", "2.10.5", "c", api, drawn)
	s.NoError(err)
	s.Equal("3.0-1", got, "the second row is the Airflow 3 Runtime")
	s.Contains(drawn.String(), "Runtime-3.0-1")
}
