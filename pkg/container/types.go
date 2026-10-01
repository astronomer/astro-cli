package container

// ListedMachine mirrors an entry from `podman machine ls --format json`.
type ListedMachine struct {
	Name    string
	Default bool
	Running bool
}

// PodmanSocket is the socket path from `podman machine inspect`.
type PodmanSocket struct {
	Path string
}

// PodmanPipe is the Windows named pipe path from `podman machine inspect`,
// e.g. `\\.\pipe\podman-machine-default`.
type PodmanPipe struct {
	Path string
}

// ConnectionInfo holds the connection details from `podman machine inspect`.
type ConnectionInfo struct {
	PodmanSocket PodmanSocket
	PodmanPipe   PodmanPipe
}

// InspectedMachine mirrors an entry from `podman machine inspect`.
type InspectedMachine struct {
	Name           string
	ConnectionInfo ConnectionInfo
	State          string
}
