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

	// CFGPostgresUser is the default postgres user
	CFGPostgresUser = "postgres.user"
	// CFGPostgresPassword is the default postgres password
	CFGPostgresPassword = "postgres.password"
	// CFGPostgresHost is the default postgres host
	CFGPostgresHost = "postgres.host"
	// CFGPostgresPort is the default postgres port
	CFGPostgresPort = "postgres.port"

	// CFGProjectName is the name of a project
	CFGProjectName = "project.name"

	// HomeConfigPath is the path to the users global directory
	HomeConfigPath = filepath.Join(utils.GetHomeDir(), ConfigDir)
	// HomeConfigFile is the global config file
	HomeConfigFile = filepath.Join(HomeConfigPath, ConfigFileNameWithExt)

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

	// Set defaults
	viperHome.SetDefault(CFGPostgresUser, "postgres")
	viperHome.SetDefault(CFGPostgresPassword, "postgres")
	viperHome.SetDefault(CFGPostgresHost, "postgres")
	viperHome.SetDefault(CFGPostgresPort, "5432")

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

// SetHomeString sets a string value in home config
func SetHomeString(config, value string) {
	if !configExists(viperHome) {
		return
	}
	viperHome.Set(config, value)
	saveConfig(viperHome, HomeConfigFile)
}

// SetProjectString sets a string value in project config
func SetProjectString(config, value string) {
	if !configExists(viperProject) {
		return
	}
	viperProject.Set(config, value)
	saveConfig(viperProject, viperProject.ConfigFileUsed())
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
	return filepath.Dir(configPath), nil
}

// GetString will return the requested config, check working dir and fallback to home
func GetString(config string) string {
	if configExists(viperProject) && viperProject.IsSet(config) {
		return viperProject.GetString(config)
	}
	return viperHome.GetString(config)
}

// saveConfig will save the config to a file
func saveConfig(v *viper.Viper, file string) error {
	err := v.WriteConfigAs(file)
	if err != nil {
		return errors.Wrap(err, "Error saving config")
	}
	return nil
}
