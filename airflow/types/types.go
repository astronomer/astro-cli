package types

// ImageBuildConfig defines options when building a container image
type ImageBuildConfig struct {
	Path            string
	TargetPlatforms []string
	NoCache         bool
	Output          bool
	SkipRevision    bool
}
