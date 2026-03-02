package types

// ImageBuildConfig defines options when building a container image
type ImageBuildConfig struct {
	Path            string
	TargetPlatforms []string
	NoCache         bool
	Labels          []string
}

// StartOptions holds mode-specific options for the Start command.
// Fields that don't apply to a given handler are silently ignored.
type StartOptions struct {
	Foreground bool   // standalone: run in the foreground
	Port       string // standalone: webserver port override
}
