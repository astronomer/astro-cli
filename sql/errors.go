package sql

import "fmt"

func EnvVarNotSetError(envVar string) error {
	return fmt.Errorf("environment variable %s not set", envVar)
}
