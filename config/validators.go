package config

import (
	"fmt"
	"strings"

	"github.com/distribution/reference"
)

// ValidateRegistryEndpoint checks a registry endpoint is a repository a deploy
// can push to: registry[:port]/repository, with no tag or digest. astro remote
// deploy checks the agent registry with it, wherever the value came from.
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
		return fmt.Errorf("registry endpoint must not include an image tag or digest, got %q; use 'registry[:port]/repository/path' without ':tag@sha256:...'", endpoint)
	case digested:
		return fmt.Errorf("registry endpoint must not include an image digest, got %q; use 'registry[:port]/repository/path' without '@sha256:...'", endpoint)
	case tagged:
		return fmt.Errorf("registry endpoint must not include an image tag, got %q; use 'registry[:port]/repository/path' without a trailing ':tag'", endpoint)
	}

	return nil
}
