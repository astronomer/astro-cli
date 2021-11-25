package play

import (
	"net"
)

//go:generate go run ../generator/generator.go KubeOptions
// KubeOptions are optional options for replaying kube YAML files
type KubeOptions struct {
	// Authfile - path to an authentication file.
	Authfile *string
	// CertDir - to a directory containing TLS certifications and keys.
	CertDir *string
	// Username for authenticating against the registry.
	Username *string
	// Password for authenticating against the registry.
	Password *string
	// Network - name of the CNI network to connect to.
	Network *string
	// Quiet - suppress output when pulling images.
	Quiet *bool
	// SignaturePolicy - path to a signature-policy file.
	SignaturePolicy *string
	// SkipTLSVerify - skip https and certificate validation when
	// contacting container registries.
	SkipTLSVerify *bool
	// SeccompProfileRoot - path to a directory containing seccomp
	// profiles.
	SeccompProfileRoot *string
	// StaticIPs - Static IP address used by the pod(s).
	StaticIPs *[]net.IP
	// StaticMACs - Static MAC address used by the pod(s).
	StaticMACs *[]net.HardwareAddr
	// ConfigMaps - slice of pathnames to kubernetes configmap YAMLs.
	ConfigMaps *[]string
	// LogDriver for the container. For example: journald
	LogDriver *string
	// Start - don't start the pod if false
	Start *bool
}
