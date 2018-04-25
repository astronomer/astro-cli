package config

// cfg defines settings a single configuration setting can have
type cfg struct {
	Path     string
	Settable bool
	Default  string
}

// cfgs houses all configurations for an astro project
type cfgs struct {
	CloudDomain	  	  cfg
	CloudAPIProtocol  cfg
	CloudAPIPort      cfg
	PostgresUser      cfg
	PostgresPassword  cfg
	PostgresHost      cfg
	PostgresPort      cfg
	RegistryAuthority cfg
	RegistryUser      cfg
	RegistryPassword  cfg
	ProjectName       cfg
	UserAPIAuthToken  cfg
}

// Creates a new cfg struct
func newCfg(path string, settable bool, dflt string) cfg {
	cfg := cfg{path, settable, dflt}
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
