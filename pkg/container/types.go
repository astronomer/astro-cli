package container

// MachineState is the lifecycle state of the Podman astro-machine.
type MachineState string

const (
	// MachineRunning means the astro-machine exists and is running.
	MachineRunning MachineState = "running"
	// MachineStopped means the astro-machine exists but is not running.
	MachineStopped MachineState = "stopped"
	// MachineAbsent means the astro-machine does not exist (and, for non-Podman
	// engines, that the concept does not apply).
	MachineAbsent MachineState = "absent"
)

// ListedMachine mirrors an entry from `podman machine ls --format json`.
type ListedMachine struct {
	Name     string
	Running  bool
	Starting bool
	LastUp   string
}

// PodmanSocket is the socket path from `podman machine inspect`.
type PodmanSocket struct {
	Path string
}

// ConnectionInfo holds the connection details from `podman machine inspect`.
type ConnectionInfo struct {
	PodmanSocket PodmanSocket
}

// InspectedMachine mirrors an entry from `podman machine inspect`.
type InspectedMachine struct {
	Name           string
	ConnectionInfo ConnectionInfo
	State          string
}

// ListedContainer mirrors an entry from `podman ps --format json`.
type ListedContainer struct {
	Name   string
	Labels map[string]string
}
