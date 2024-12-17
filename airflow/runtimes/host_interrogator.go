package runtimes

type HostInterrogator interface {
	IsMac() bool
	IsWindows() bool
	FileExists(string) bool
	GetEnvVar(string) string
}

type hostInterrogator struct {
	OSChecker
	FileChecker
	EnvChecker
}

func CreateHostInspector(osChecker OSChecker, fileChecker FileChecker, envChecker EnvChecker) HostInterrogator {
	return hostInterrogator{
		osChecker,
		fileChecker,
		envChecker,
	}
}

func CreateHostInspectorWithDefaults() HostInterrogator {
	return hostInterrogator{
		CreateOSChecker(),
		CreateFileChecker(),
		CreateEnvChecker(),
	}
}
