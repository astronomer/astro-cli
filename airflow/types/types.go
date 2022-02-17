package types

// ContainerStartConfig defines options when starting a container project
type ContainerStartConfig struct {
	DockerfilePath string
	NoCache        bool
}

// ImageBuildConfig defines options when building a container image
type ImageBuildConfig struct {
	Path    string
	NoCache bool
}
