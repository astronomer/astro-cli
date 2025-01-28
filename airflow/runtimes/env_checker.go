package runtimes

import (
	"os"
)

type EnvChecker interface {
	GetEnvVar(string) string
}

type envChecker struct{}

func CreateEnvChecker() EnvChecker {
	return new(envChecker)
}

// GetEnvVar returns the value of the specified environment variable.
func (o envChecker) GetEnvVar(string) string {
	return os.Getenv("PATH")
}
