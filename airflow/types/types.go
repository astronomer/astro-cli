package types

import (
	"time"

	"github.com/astronomer/astro-cli/astro-client-v1"
)

// ImageBuildConfig defines options when building a container image
type ImageBuildConfig struct {
	Path            string
	TargetPlatforms []string
	NoCache         bool
	Labels          []string
}

// StartOptions holds all options for the Start command.
// Fields that don't apply to a given handler are silently ignored.
type StartOptions struct {
	// Common options (used by both Docker and Standalone)
	ImageName         string
	SettingsFile      string
	ComposeFile       string
	BuildSecretString string
	NoCache           bool
	NoBrowser         bool
	WaitTime          time.Duration
	EnvConns          map[string]astrov1.EnvironmentObjectConnection

	// Proxy options
	NoProxy bool // disable the reverse proxy (use fixed ports instead)

	// Standalone-specific options (ignored by DockerCompose)
	Foreground bool   // standalone: run in the foreground
	Port       string // standalone: webserver port override
}
