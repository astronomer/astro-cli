package config

import "errors"

var (
	ErrHomeConfigNotFound    = errors.New("home config doesn't exists")
	ErrProjectConfigNotFound = errors.New("project config doesn't exists")
)

// cfg defines settings a single configuration setting can have
type cfg struct {
	Path    string
	Default string
}

// cfgs houses all configurations for an astro project
type cfgs struct {
	CloudAPIProtocol       cfg
	CloudAPIPort           cfg
	CloudWSProtocol        cfg
	CloudAPIToken          cfg
	Context                cfg
	Contexts               cfg
	LocalEnabled           cfg
	LocalHouston           cfg
	LocalOrbit             cfg
	PostgresUser           cfg
	PostgresPassword       cfg
	PostgresHost           cfg
	PostgresPort           cfg
	ProjectName            cfg
	ProjectDeployment      cfg
	ProjectWorkspace       cfg
	WebserverPort          cfg
	ShowWarnings           cfg
	AirflowReleasesURL     cfg
	SkipVerifyTLS          cfg
	Verbosity              cfg
	ContainerEngine        cfg
	PodmanConnectionURI    cfg
	SchedulerContainerName cfg
	WebserverContainerName cfg
	TriggererContainerName cfg
	HoustonDialTimeout     cfg
}

// Creates a new cfg struct
func newCfg(path, dflt string) cfg {
	config := cfg{path, dflt}
	CFGStrMap[path] = config
	return config
}

// SetHomeString sets a string value in home config
func (c cfg) SetHomeString(value string) error {
	if !configExists(viperHome) {
		return ErrHomeConfigNotFound
	}
	viperHome.Set(c.Path, value)
	err := saveConfig(viperHome, HomeConfigFile)
	return err
}

// SetProjectString sets a string value in project config
func (c cfg) SetProjectString(value string) error {
	if !configExists(viperProject) {
		return ErrProjectConfigNotFound
	}
	viperProject.Set(c.Path, value)
	err := saveConfig(viperProject, viperProject.ConfigFileUsed())
	return err
}

// GetString will return the requested config, check working dir and fallback to home
func (c cfg) GetString() string {
	if configExists(viperProject) && viperProject.IsSet(c.Path) {
		return c.GetProjectString()
	}
	return c.GetHomeString()
}

// GetBool will return the requested config, check working dir and fallback to home
func (c cfg) GetBool() bool {
	if configExists(viperProject) && viperProject.IsSet(c.Path) {
		return viperProject.GetBool(c.Path)
	}
	return viperHome.GetBool(c.Path)
}

// GetBool will return the integer value of requested config, check working dir and fallback to home
func (c cfg) GetInt() int {
	if configExists(viperProject) && viperProject.IsSet(c.Path) {
		return viperProject.GetInt(c.Path)
	}
	return viperHome.GetInt(c.Path)
}

// GetProjectString will return a project config
func (c cfg) GetProjectString() string {
	return viperProject.GetString(c.Path)
}

// GetHomeString will return config from home string
func (c cfg) GetHomeString() string {
	return viperHome.GetString(c.Path)
}
