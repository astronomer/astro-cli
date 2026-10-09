package airflow

import (
	"github.com/pkg/errors"
)

var errMockDocker = errors.New("mock docker compose error")

func (s *Suite) TestRepositoryName() {
	s.Equal(repositoryName("test-repo"), "test-repo/airflow")
}

func (s *Suite) TestImageName() {
	s.Equal(ImageName("test-repo", "0.15.0"), "test-repo/airflow:0.15.0")
}
