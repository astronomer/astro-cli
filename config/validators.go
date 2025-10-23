package config

import (
	"fmt"
	"strings"
)

// registerValidator is a helper to register a validator for a config option
func registerValidator(configPath string, validator ValidatorFunc) {
	cfg := CFGStrMap[configPath]
	cfg.RegisterValidator(validator)
	CFGStrMap[configPath] = cfg
}

// registerValidators registers validation functions for config options
func registerValidators() {
	registerValidator("remote.client_registry", ValidateRegistryEndpoint)
}

// ValidateRegistryEndpoint validates a registry endpoint format
func ValidateRegistryEndpoint(endpoint string) error {
	if endpoint == "" {
		return fmt.Errorf("registry endpoint cannot be empty")
	}

	// Basic validation: should contain at least one slash (registry/repository format)
	if !strings.Contains(endpoint, "/") {
		return fmt.Errorf("registry endpoint must be in format 'registry/repository' (e.g. quay.io/acme/my-deployment-image)")
	}

	// Should not contain spaces
	if strings.Contains(endpoint, " ") {
		return fmt.Errorf("registry endpoint cannot contain spaces")
	}

	// Should not start or end with slashes
	if strings.HasPrefix(endpoint, "/") || strings.HasSuffix(endpoint, "/") {
		return fmt.Errorf("registry endpoint cannot start or end with slashes")
	}

	return nil
}
