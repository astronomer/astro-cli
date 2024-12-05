package config

// cfg defines settings a single configuration setting can have
type cfg struct {
	Path    string
	Default string
}

// cfgs houses all configurations for an astro project
type cfgs struct {
	CloudAPIProtocol      cfg
	CloudAPIPort          cfg
	CloudWSProtocol       cfg
	CloudAPIToken         cfg
	Context               cfg
	Contexts              cfg
	DockerCommand         cfg
	LocalAstro            cfg
	LocalCore             cfg
	LocalPublicAstro      cfg
	LocalRegistry         cfg
	LocalHouston          cfg
	LocalPlatform         cfg
	DuplicateImageVolumes cfg
	PostgresUser          cfg
	PostgresPassword      cfg
	PostgresHost          cfg
	PostgresPort          cfg
	PostgresRepository    cfg
	PostgresTag           cfg
	ProjectName           cfg
	ProjectDeployment     cfg
	ProjectWorkspace      cfg
	WebserverPort         cfg
	AirflowExposePort     cfg
	ShowWarnings          cfg
	Verbosity             cfg
	HoustonDialTimeout    cfg
	HoustonSkipVerifyTLS  cfg
	SkipParse             cfg
	Interactive           cfg
	PageSize              cfg
	AuditLogs             cfg
	UpgradeMessage        cfg
	DisableAstroRun       cfg
	DisableEnvObjects     cfg
	AutoSelect            cfg
	PodmanCPU             cfg
	PodmanMemory          cfg
}

// Creates a new cfg struct
func newCfg(path, dflt string) cfg {
	ncfg := cfg{path, dflt}
	CFGStrMap[path] = ncfg
	return ncfg
}

// SetHomeString sets a string value in home config
func (c cfg) SetHomeString(value string) error {
	if !configExists(viperHome) {
		return nil
	}
	viperHome.Set(c.Path, value)
	err := saveConfig(viperHome, HomeConfigFile)
	if err != nil {
		return err
	}
	return nil
}

// SetProjectString sets a string value in project config
func (c cfg) SetProjectString(value string) error {
	if !configExists(viperProject) {
		return nil
	}
	viperProject.Set(c.Path, value)
	err := saveConfig(viperProject, viperProject.ConfigFileUsed())
	if err != nil {
		return err
	}
	return nil
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

// GetInt will return the integer value of requested config, check working dir and fallback to home
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
