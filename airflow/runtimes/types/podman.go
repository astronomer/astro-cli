package types

// ListedMachine contains information about a Podman machine
// as it is provided from the `podman machine ls --format json` command.
type ListedMachine struct {
	Name     string
	Running  bool
	Starting bool
	LastUp   string
}

// PodmanSocket contains the path to the Podman socket.
type PodmanSocket struct {
	Path string
}

// ConnectionInfo contains information about the connection to a Podman machine.
type ConnectionInfo struct {
	PodmanSocket PodmanSocket
}

// InspectedMachine contains information about a Podman machine
// as it is provided from the `podman machine inspect` command.
type InspectedMachine struct {
	Name           string
	ConnectionInfo ConnectionInfo
	State          string
}

// ListedContainer contains information about a Podman container
// as it is provided from the `podman ps --format json` command.
type ListedContainer struct {
	Name   string
	Labels map[string]string
}
