package localdocker

import (
	"context"
	"errors"
	"strings"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// stubImages stands in for pkg/imagebuild. The tests here are about the engine's
// compose and lifecycle behavior, not about building images, and the real builder
// cannot be imported anyway — see rt.ImageBuilder.
type stubImages struct {
	base         string
	airflow2Base string
	built        string
	err          error
	requests     []rt.BuildRequest
}

func newStubImages() *stubImages {
	return &stubImages{
		base:         "astrocrpublic.azurecr.io/runtime:3.0-1",
		airflow2Base: "quay.io/astronomer/astro-runtime:13.9.0",
	}
}

// RuntimeImage mirrors the real builder's rules closely enough for the engine's
// tests: both generations resolve, an Airflow 2 image comes from the other
// repository, and anything else is refused. The engine's contribution is
// surfacing a refusal before anything starts, which is what
// TestStartRejectsNonDockerPlanAndBadVersions checks; the rules themselves —
// the 2.7 floor, and the version-service lookup an Airflow 2 pin needs — belong
// to pkg/imagebuild and are tested there.
func (s *stubImages) RuntimeImage(_ context.Context, airflowVersion string) (string, error) {
	switch {
	case strings.HasPrefix(airflowVersion, "3"):
		return s.base, nil
	case strings.HasPrefix(airflowVersion, "2"):
		return s.airflow2Base, nil
	default:
		return "", errors.New("Docker mode runs Airflow 2 or Airflow 3, not " + airflowVersion)
	}
}

func (s *stubImages) Build(_ context.Context, req rt.BuildRequest, _ rt.Callbacks) (string, error) {
	s.requests = append(s.requests, req)
	if s.err != nil {
		return "", s.err
	}
	if s.built != "" {
		return s.built, nil
	}
	// Matches the real builder's no-op case: nothing to install, so the base
	// image is what runs.
	if len(req.Dependencies) == 0 && len(req.Packages) == 0 {
		return req.BaseImage, nil
	}
	return req.Tag, nil
}
