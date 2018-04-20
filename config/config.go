package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/astronomerio/astro-cli/utils"
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
	HomeConfigPath = filepath.Join(utils.GetHomeDir(), ConfigDir)
	// HomeConfigFile is the global config file
	HomeConfigFile = filepath.Join(HomeConfigPath, ConfigFileNameWithExt)

	// CFGStrMap maintains string to cfg mapping
	CFGStrMap = make(map[string]cfg)

	// CFG Houses configuration meta
	CFG = cfgs{
		PostgresUser:      newCfg("postgres.user", true, "postgres"),
		PostgresPassword:  newCfg("postgres.password", true, "postgres"),
		PostgresHost:      newCfg("postgres.host", true, "postgres"),
		PostgresPort:      newCfg("postgres.port", true, "5432"),
		RegistryAuthority: newCfg("docker.registry.authority", true, ""),
		RegistryAuth:      newCfg("docker.registry.auth", true, ""),
		ProjectName:       newCfg("project.name", true, ""),
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

// FetchDecodedAuth fetches auth string from config, decodes and
// returns username password
func FetchDecodedAuth() (string, string, error) {
	encodedAuth := CFG.RegistryAuth.GetString()
	return DecodeAuth(encodedAuth)
}

// EncodeAuth creates a base64 encoded string to containing authorization information
func EncodeAuth(username, password string) string {
	authStr := username + ":" + password
	msg := []byte(authStr)
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(msg)))
	base64.StdEncoding.Encode(encoded, msg)
	return string(encoded)
}

// DecodeAuth decodes a base64 encoded string and returns username and password
func DecodeAuth(authStr string) (string, string, error) {
	if authStr == "" {
		return "", "", nil
	}

	decLen := base64.StdEncoding.DecodedLen(len(authStr))
	decoded := make([]byte, decLen)
	authByte := []byte(authStr)
	n, err := base64.StdEncoding.Decode(decoded, authByte)
	if err != nil {
		return "", "", err
	}
	if n > decLen {
		return "", "", errors.Errorf("Something went wrong decoding auth config")
	}
	arr := strings.SplitN(string(decoded), ":", 2)
	if len(arr) != 2 {
		return "", "", errors.Errorf("Invalid auth configuration file")
	}
	password := strings.Trim(arr[1], "\x00")
	return arr[0], password, nil
}
