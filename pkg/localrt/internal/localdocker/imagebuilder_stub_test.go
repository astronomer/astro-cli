package localdocker

import (
	"context"
	"errors"
	"strings"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/rt"
)

// stubImages stands in for pkg/imagebuild. The tests here are about the engine's
// compose and lifecycle behavior, not about building images, and the real builder
// cannot be imported anyway — see rt.ImageBuilder.
type stubImages struct {
	base     string
	built    string
	err      error
	requests []rt.BuildRequest
}

func newStubImages() *stubImages {
	return &stubImages{base: "astrocrpublic.azurecr.io/runtime:3.0-1"}
}

// RuntimeImage mirrors the real builder's one rule: Astro Runtime images are
// Airflow 3 only. The engine's contribution is surfacing that refusal before
// anything starts, which is what TestStartRejectsNonDockerPlanAndBadVersions
// checks; the rule itself belongs to pkg/imagebuild and is tested there.
func (s *stubImages) RuntimeImage(airflowVersion string) (string, error) {
	if strings.HasPrefix(airflowVersion, "2.") {
		return "", errors.New("local Docker mode needs Airflow 3")
	}
	return s.base, nil
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
