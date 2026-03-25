package types

// ContainerStatus represents the status of a single container
type ContainerStatus struct {
	Name  string   `json:"name"`
	State string   `json:"state"`
	Ports []string `json:"ports,omitempty"`
}

// PSStatus represents the overall status of the development environment
type PSStatus struct {
	// For Docker mode: list of containers
	Containers []ContainerStatus `json:"containers,omitempty"`
	// For standalone mode: process information
	Running *bool  `json:"running,omitempty"`
	PID     int    `json:"pid,omitempty"`
	Mode    string `json:"mode"` // "docker" or "standalone"
}
