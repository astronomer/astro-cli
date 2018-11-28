package config

// cfg defines settings a single configuration setting can have
type cfg struct {
	Path    string
	Default string
}

// cfgs houses all configurations for an astro project
type cfgs struct {
	CloudAPIProtocol  cfg
	CloudAPIPort      cfg
	CloudAPIToken     cfg
	Context           cfg
	Contexts          cfg
	LocalEnabled      cfg
	LocalHouston      cfg
	LocalOrbit        cfg
	PostgresUser      cfg
	PostgresPassword  cfg
	PostgresHost      cfg
	PostgresPort      cfg
	ProjectName       cfg
	ProjectDeployment cfg
	ProjectWorkspace  cfg
	WebserverPort     cfg
	PrivateKey        cfg
}

// Creates a new cfg struct
func newCfg(path string, dflt string) cfg {
	cfg := cfg{path, dflt}
	CFGStrMap[path] = cfg
	return cfg
}

// SetHomeString sets a string value in home config
func (c cfg) SetHomeString(value string) {
	if !configExists(viperHome) {
		return
	}
	viperHome.Set(c.Path, value)
	saveConfig(viperHome, HomeConfigFile)
}

// SetProjectString sets a string value in project config
func (c cfg) SetProjectString(value string) {
	if !configExists(viperProject) {
		return
	}
	viperProject.Set(c.Path, value)
	saveConfig(viperProject, viperProject.ConfigFileUsed())
}

// GetString will return the requested config, check working dir and fallback to home
func (c cfg) GetString() string {
	if configExists(viperProject) && viperProject.IsSet(c.Path) {
		return c.GetProjectString()
	}
	return c.GetHomeString()
}

// GetProjectString will return a project config
func (c cfg) GetProjectString() string {
	return viperProject.GetString(c.Path)
}

// GetHomeString will return config from home string
func (c cfg) GetHomeString() string {
	return viperHome.GetString(c.Path)
}
