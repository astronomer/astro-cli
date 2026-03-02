package types

import (
	"time"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
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
	EnvConns          map[string]astrocore.EnvironmentObjectConnection

	// Standalone-specific options (ignored by DockerCompose)
	Foreground bool   // standalone: run in the foreground
	Port       string // standalone: webserver port override
}
