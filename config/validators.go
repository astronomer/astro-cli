package config

import (
	"fmt"
	"strings"

	"github.com/distribution/reference"
)

// registerValidator is a helper to register a validator for a config option
func registerValidator(configPath string, validator ValidatorFunc) {
	c := CFGStrMap[configPath]
	c.RegisterValidator(validator)
	CFGStrMap[configPath] = c
}

// registerValidators registers validation functions for config options
func registerValidators() {
	registerValidator(CFG.RemoteClientRegistry.Path, ValidateRegistryEndpoint)
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

	// The rest is Docker's own grammar for an image name, so what passes here
	// is what a deploy can push: lowercase, legal characters, a numeric port on
	// the registry host if any, and no tag or digest. The CLI appends its own
	// "deploy-<timestamp>" tag, so a tag or digest here would make an invalid
	// reference that Docker reports only as "invalid reference format".
	ref, err := reference.ParseNormalizedNamed(endpoint)
	if err != nil {
		return fmt.Errorf("registry endpoint %q is not a valid image repository: %w", endpoint, err)
	}
	_, tagged := ref.(reference.Tagged)
	_, digested := ref.(reference.Digested)
	switch {
	case tagged && digested:
		return fmt.Errorf("registry endpoint must not include an image tag or digest -- got %q, use 'registry[:port]/repository/path' without the ':tag@sha256:...' (the CLI appends its own deploy tag automatically)", endpoint)
	case digested:
		return fmt.Errorf("registry endpoint must not include an image digest -- got %q, use 'registry[:port]/repository/path' without '@sha256:...' (the CLI appends its own deploy tag automatically)", endpoint)
	case tagged:
		return fmt.Errorf("registry endpoint must not include an image tag -- got %q, use 'registry[:port]/repository/path' without a trailing ':tag' (the CLI appends its own deploy tag automatically)", endpoint)
	}

	return nil
}

// ClientRegistry is deploy.remote_execution.client_registry (1.x's
// remote.client_registry) as the CLI uses it: the project
// config's when it sets one, else the global config's; "" when neither does.
// A saved value is checked as a new one is, since it may predate the check or
// have been written into the file by hand, and one that is not a registry
// endpoint a deploy can push to is an error naming the config that holds it
// and the command that fixes it there.
func ClientRegistry() (string, error) {
	value := CFG.RemoteClientRegistry.GetString()
	if value == "" {
		return "", nil
	}
	if err := ValidateRegistryEndpoint(value); err != nil {
		where, fix := clientRegistryScope()
		return "", fmt.Errorf("%s in %s is not a usable registry endpoint: %w. Fix it with: '%s'", clientRegistryKey(), where, err, fix)
	}
	return value, nil
}

// ClientRegistryFix is the command that sets the client registry where the
// CLI reads it from, for an error that says to set it.
func ClientRegistryFix() string {
	_, fix := clientRegistryScope()
	return fix
}

// clientRegistryScope names the config the client registry is read from and
// the command that changes it there: the project's when it sets the key,
// even to "", since that shadows the global one; else the global one, which
// takes -g from anywhere: without it, `astro config set` writes the project's
// config inside a project and refuses outside one. The command names the
// current key, which wins over the 1.x one it was read from.
func clientRegistryScope() (where, fix string) {
	c := CFG.RemoteClientRegistry
	if c.projectKey() != "" {
		return "this project's config", "astro config set " + c.Path + " <registry>/<repository>"
	}
	return "your global config", "astro config set -g " + c.Path + " <registry>/<repository>"
}

// clientRegistryKey is the name the config holding the client registry sets
// it under, as the user would find it in the file: the current name, or the
// 1.x one.
func clientRegistryKey() string {
	c := CFG.RemoteClientRegistry
	if key := c.projectKey(); key != "" {
		return key
	}
	if key := c.keyIn(viperHome); key != "" {
		return key
	}
	return c.Path
}
