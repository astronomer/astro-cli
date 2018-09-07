package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/astronomerio/astro-cli/messages"
	"github.com/astronomerio/astro-cli/pkg/fileutil"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

var (
	// ConfigFileName is the name of the config files (home / project)
	ConfigFileName = "config"
	// ConfigFileType is the config file extension
	ConfigFileType = "yaml"
	// ConfigFileNameWithExt is the config filename with extension
	ConfigFileNameWithExt = fmt.Sprintf("%s.%s", ConfigFileName, ConfigFileType)
	// ConfigDir is the directory for astro files
	ConfigDir = ".astro"

	// HomePath is the path to a users home directory
	HomePath, _ = fileutil.GetHomeDir()
	// HomeConfigPath is the path to the users global config directory
	HomeConfigPath = filepath.Join(HomePath, ConfigDir)
	// HomeConfigFile is the global config file
	HomeConfigFile = filepath.Join(HomeConfigPath, ConfigFileNameWithExt)

	// WorkingPath is the path to the working directory
	WorkingPath, _ = fileutil.GetWorkingDir()

	// CFGStrMap maintains string to cfg mapping
	CFGStrMap = make(map[string]cfg)

	// CFG Houses configuration meta
	CFG = cfgs{
		Contexts:          newCfg("contexts", ""),
		CloudAPIProtocol:  newCfg("cloud.api.protocol", "https"),
		CloudAPIPort:      newCfg("cloud.api.port", "443"),
		CloudAPIToken:     newCfg("cloud.api.token", ""),
		Context:           newCfg("context", ""),
		LocalHouston:      newCfg("local.houston", ""),
		LocalOrbit:        newCfg("local.orbit", ""),
		PostgresUser:      newCfg("postgres.user", "postgres"),
		PostgresPassword:  newCfg("postgres.password", "postgres"),
		PostgresHost:      newCfg("postgres.host", "postgres"),
		PostgresPort:      newCfg("postgres.port", "5432"),
		ProjectDeployment: newCfg("project.deployment", ""),
		ProjectName:       newCfg("project.name", ""),
		ProjectWorkspace:  newCfg("project.workspace", ""),
	}

	// viperHome is the viper object in the users home directory
	viperHome *viper.Viper
	// viperProject is the viper object in a project directory
	viperProject *viper.Viper
)

// InitConfig initializes the config files
func InitConfig() {
	initHome()
	initProject()
}

// Init viper for config file in home directory
func initHome() {
	viperHome = viper.New()
	viperHome.SetConfigName(ConfigFileName)
	viperHome.SetConfigType(ConfigFileType)
	viperHome.SetConfigFile(HomeConfigFile)

	for _, cfg := range CFGStrMap {
		if len(cfg.Default) > 0 {
			viperHome.SetDefault(cfg.Path, cfg.Default)
		}
	}

	// If home config does not exist, create it
	homeConfigExists, _ := fileutil.Exists(HomeConfigFile)

	if !homeConfigExists {
		err := CreateConfig(viperHome, HomeConfigPath, HomeConfigFile)
		if err != nil {
			fmt.Printf(messages.CONFIG_CREATE_HOME_ERROR, err)
			return
		}
	}

	// Read in home config
	err := viperHome.ReadInConfig()
	if err != nil {
		fmt.Printf(messages.CONFIG_READ_ERROR, err)
		return
	}
}

// Init viper for config file in project directory
// If project config does not exist, just exit
func initProject() {
	// Set up viper object for project config
	viperProject = viper.New()
	viperProject.SetConfigName(ConfigFileName)
	viperProject.SetConfigType(ConfigFileType)

	// Construct the path to the config file
	workingConfigPath := filepath.Join(WorkingPath, ConfigDir)

	workingConfigFile := filepath.Join(workingConfigPath, ConfigFileNameWithExt)

	// If path is empty or config file does not exist, just return
	workingConfigExists, _ := fileutil.Exists(workingConfigFile)
	if len(workingConfigPath) == 0 || workingConfigPath == HomeConfigPath || !workingConfigExists {
		return
	}

	// Add the path we discovered
	viperProject.SetConfigFile(workingConfigFile)

	// Read in project config
	readErr := viperProject.ReadInConfig()
	if readErr != nil {
		fmt.Printf(messages.CONFIG_READ_ERROR, readErr)
	}
}

// CreateProjectConfig creates a project config file
func CreateProjectConfig(projectPath string) {
	projectConfigDir := filepath.Join(projectPath, ConfigDir)
	projectConfigFile := filepath.Join(projectConfigDir, ConfigFileNameWithExt)

	err := CreateConfig(viperProject, projectConfigDir, projectConfigFile)
	if err != nil {
		fmt.Printf(messages.CONFIG_CREATE_HOME_ERROR, err)
		return
	}

	// Add the new file
	viperProject.SetConfigFile(projectConfigFile)
}

// configExists returns a boolean indicating if the config is backed by a file
func configExists(v *viper.Viper) bool {
	return len(v.ConfigFileUsed()) > 0
}

// CreateConfig creates a config file in the given directory
func CreateConfig(v *viper.Viper, path, file string) error {
	err := os.MkdirAll(path, 0770)
	if err != nil {
		return errors.Wrap(err, messages.CONFIG_CREATE_DIR_ERROR)
	}

	_, err = os.Create(file)
	if err != nil {
		return errors.Wrap(err, messages.CONFIG_CREATE_FILE_ERROR)
	}
	os.Chmod(file, 0600)

	return saveConfig(v, file)
}

// ProjectConfigExists returns a boolean indicating if a project config file exists
func ProjectConfigExists() bool {
	return configExists(viperProject)
}

// ProjectRoot returns the path to the nearest project root
// TODO Deprecate if remains unused, removed due to
// https://github.com/astronomerio/astro-cli/issues/103
func ProjectRoot() (string, error) {
	configPath, searchErr := fileutil.FindDirInPath(ConfigDir)
	if searchErr != nil {
		return "", searchErr
	}
	if len(configPath) == 0 {
		return "", nil
	}
	return filepath.Dir(configPath), nil
}

// IsProjectDir returns a boolean depending on if path is a valid project dir
func IsProjectDir(path string) (bool, error) {
	configPath := filepath.Join(path, ConfigDir)
	configFile := filepath.Join(configPath, ConfigFileNameWithExt)

	// Home directory is not a project directory
	if HomePath == path {
		return false, nil
	}

	return fileutil.Exists(configFile)
}

// saveConfig will save the config to a file
func saveConfig(v *viper.Viper, file string) error {
	err := v.WriteConfigAs(file)
	if err != nil {
		return errors.Wrap(err, messages.CONFIG_SAVE_ERROR)
	}
	return nil
}
