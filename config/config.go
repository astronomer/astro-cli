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

	// HomeConfigPath is the path to the users global directory
	HomeConfigPath = filepath.Join(fileutil.GetHomeDir(), ConfigDir)
	// HomeConfigFile is the global config file
	HomeConfigFile = filepath.Join(HomeConfigPath, ConfigFileNameWithExt)

	// CFGStrMap maintains string to cfg mapping
	CFGStrMap = make(map[string]cfg)

	// CFG Houses configuration meta
	CFG = cfgs{
		Contexts:         newCfg("clusters", ""),
		CloudDomain:      newCfg("cloud.domain", ""),
		CloudAPIProtocol: newCfg("cloud.api.protocol", "https"),
		CloudAPIPort:     newCfg("cloud.api.port", "443"),
		CloudAPIToken:    newCfg("cloud.api.token", ""),
		Context:          newCfg("context", ""),
		LocalAPIURL:      newCfg("local.api.url", ""),
		PostgresUser:     newCfg("postgres.user", "postgres"),
		PostgresPassword: newCfg("postgres.password", "postgres"),
		PostgresHost:     newCfg("postgres.host", "postgres"),
		PostgresPort:     newCfg("postgres.port", "5432"),
		ProjectName:      newCfg("project.name", ""),
		ProjectWorkspace: newCfg("project.workspace", ""),
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
	if !fileutil.Exists(HomeConfigFile) {
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

	configPath, searchErr := fileutil.FindDirInPath(ConfigDir)
	if searchErr != nil {
		fmt.Printf(messages.CONFIG_SEARCH_ERROR+"\n", searchErr)
		return
	}

	// Construct the path to the config file
	projectConfigFile := filepath.Join(configPath, ConfigFileNameWithExt)

	// If path is empty or config file does not exist, just return
	if len(configPath) == 0 || configPath == HomeConfigPath || !fileutil.Exists(projectConfigFile) {
		return
	}

	// Add the path we discovered
	viperProject.SetConfigFile(projectConfigFile)

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

// saveConfig will save the config to a file
func saveConfig(v *viper.Viper, file string) error {
	err := v.WriteConfigAs(file)
	if err != nil {
		return errors.Wrap(err, messages.CONFIG_SAVE_ERROR)
	}
	return nil
}

func getUrl(svc string) string {
	return fmt.Sprintf(
		"%s://%s.%s:%s/",
		CFG.CloudAPIProtocol.GetString(),
		svc,
		CFG.CloudDomain.GetString(),
		CFG.CloudAPIPort.GetString(),
	)
}

// APIUrl will return a full qualified API url
func APIUrl() string {
	if len(CFG.LocalAPIURL.GetString()) != 0 {
		return CFG.LocalAPIURL.GetString()
	} else {
		return getUrl("houston") + "v1"
	}
}
