package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/astronomerio/astro-cli/utils"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

var (
	// ConfigFileName is the name of the config files (home / project)
	ConfigFileName = "config"
	// ConfigFileType is the config file extension
	ConfigFileType = "json"
	// ConfigFileNameWithExt is the config filename with extension
	ConfigFileNameWithExt = fmt.Sprintf("%s.%s", ConfigFileName, ConfigFileType)
	// ConfigDir is the directory for astro files
	ConfigDir = ".astro"

	// HomeConfigPath is the path to the users global directory
	HomeConfigPath = filepath.Join(utils.GetHomeDir(), ConfigDir)
	// HomeConfigFile is the global config file
	HomeConfigFile = filepath.Join(HomeConfigPath, ConfigFileNameWithExt)

	// CFGStrMap maintains string to cfg mapping
	CFGStrMap = make(map[string]cfg)

	// CFG Houses configuration meta
	CFG = cfgs{
		PostgresUser:      initCfg("postgres.user", true, true),
		PostgresPassword:  initCfg("postgres.password", true, true),
		PostgresHost:      initCfg("postgres.host", true, true),
		PostgresPort:      initCfg("postgres.port", true, true),
		RegistryAuthority: initCfg("docker.registry.authority", true, true),
		RegistryUser:      initCfg("docker.registry.user", true, true),
		RegistryPassword:  initCfg("docker.registry.password", true, true),
		ProjectName:       initCfg("project.name", true, true),
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

func initCfg(path string, gettable bool, settable bool) cfg {
	cfg := cfg{path, gettable, settable}
	CFGStrMap[path] = cfg
	return cfg
}

// Init viper for config file in home directory
func initHome() {
	viperHome = viper.New()
	viperHome.SetConfigName(ConfigFileName)
	viperHome.SetConfigType(ConfigFileType)
	viperHome.SetConfigFile(HomeConfigFile)

	// Set defaults
	viperHome.SetDefault(CFG.PostgresUser.Path, "postgres")
	viperHome.SetDefault(CFG.PostgresPassword.Path, "postgres")
	viperHome.SetDefault(CFG.PostgresHost.Path, "postgres")
	viperHome.SetDefault(CFG.PostgresPort.Path, "5432")
	// XXX: Change default to hosted cloud, allow to be set by user for EE
	viperHome.SetDefault(CFG.RegistryAuthority.Path, "registry.gcp.astronomer.io")
	viperHome.SetDefault(CFG.RegistryUser.Path, "admin")
	viperHome.SetDefault(CFG.RegistryPassword.Path, "admin")

	// If home config does not exist, create it
	if !utils.Exists(HomeConfigFile) {
		err := CreateConfig(viperHome, HomeConfigPath, HomeConfigFile)
		if err != nil {
			fmt.Printf("Error creating default config in home dir: %s", err)
			return
		}
	}

	// Read in home config
	err := viperHome.ReadInConfig()
	if err != nil {
		fmt.Printf("Error reading config in home dir: %s", err)
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

	configPath, searchErr := utils.FindDirInPath(ConfigDir)
	if searchErr != nil {
		fmt.Printf("Error searching for project dir: %v\n", searchErr)
		return
	}

	// Construct the path to the config file
	projectConfigFile := filepath.Join(configPath, ConfigFileNameWithExt)

	// If path is empty or config file does not exist, just return
	if len(configPath) == 0 || configPath == HomeConfigPath || !utils.Exists(projectConfigFile) {
		return
	}

	// Add the path we discovered
	viperProject.SetConfigFile(projectConfigFile)

	// Read in project config
	readErr := viperProject.ReadInConfig()
	if readErr != nil {
		fmt.Printf("Error reading config in project dir: %s", readErr)
	}
}

// CreateProjectConfig creates a project config file
func CreateProjectConfig(projectPath string) {
	projectConfigDir := filepath.Join(projectPath, ConfigDir)
	projectConfigFile := filepath.Join(projectConfigDir, ConfigFileNameWithExt)

	err := CreateConfig(viperProject, projectConfigDir, projectConfigFile)
	if err != nil {
		fmt.Printf("Error creating default config in project dir: %s", err)
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
		return errors.Wrap(err, "Error creating config directory")
	}

	_, err = os.Create(file)
	if err != nil {
		return errors.Wrap(err, "Error creating config file")
	}
	os.Chmod(file, 0600)

	return saveConfig(v, file)
}

// ProjectConfigExists returns a boolean indicating if a project config file exists
func ProjectConfigExists() bool {
	return configExists(viperProject)
}

// ProjectRoot returns the path to the nearest project root
func ProjectRoot() (string, error) {
	configPath, searchErr := utils.FindDirInPath(ConfigDir)
	if searchErr != nil {
		return "", searchErr
	}
	if len(configPath) == 0 {
		return "", nil
	}
	return filepath.Dir(configPath), nil
}

// saveConfig will save the config to a file
func saveConfig(v *viper.Viper, file string) error {
	err := v.WriteConfigAs(file)
	if err != nil {
		return errors.Wrap(err, "Error saving config")
	}
	return nil
}
