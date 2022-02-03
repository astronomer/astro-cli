package airflow

import "errors"

var (
	errGetImageLabel                = errors.New("error getting image label")
	errComposeProjectRunning        = errors.New("project is up and running")
	errHealthCheckBreakPointReached = errors.New("reached the maximum number of health check limit")
)
