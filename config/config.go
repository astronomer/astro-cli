package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

const (
	CloudPlatform    = "cloud"
	SoftwarePlatform = "software"
	PrPreview        = "prprievew"

	localhostDomain = "localhost"
	astrohubDomain  = "astrohub"
	cloudDomain     = "cloud"
	houstonDomain   = "houston"

	configCreateHomeErrorMsg = "Error creating default config in home dir: %s"
	configReadErrorMsg       = "Error reading config in home dir: %s\n"
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
		CloudAPIProtocol:      newCfg("cloud.api.protocol", "https"),
		CloudAPIPort:          newCfg("cloud.api.port", "443"),
		CloudWSProtocol:       newCfg("cloud.api.ws_protocol", "wss"),
		CloudAPIToken:         newCfg("cloud.api.token", ""),
		Context:               newCfg("context", ""),
		Contexts:              newCfg("contexts", ""),
		DockerCommand:         newCfg("container.binary", "docker"),
		LocalAstro:            newCfg("local.astrohub", "http://localhost:8871/v1"),
		LocalCore:             newCfg("local.core", "http://localhost:8888/v1alpha1"),
		LocalPublicAstro:      newCfg("local.public_astrohub", "http://localhost:8871/graphql"),
		LocalRegistry:         newCfg("local.registry", "localhost:5555"),
		LocalHouston:          newCfg("local.houston", ""),
		LocalPlatform:         newCfg("local.platform", CloudPlatform),
		PostgresUser:          newCfg("postgres.user", "postgres"),
		PostgresPassword:      newCfg("postgres.password", "postgres"),
		PostgresHost:          newCfg("postgres.host", "postgres"),
		PostgresPort:          newCfg("postgres.port", "5432"),
		PostgresRepository:    newCfg("postgres.repository", "docker.io/postgres"),
		PostgresTag:           newCfg("postgres.tag", "12.6"),
		ProjectDeployment:     newCfg("project.deployment", ""),
		ProjectName:           newCfg("project.name", ""),
		ProjectWorkspace:      newCfg("project.workspace", ""),
		WebserverPort:         newCfg("webserver.port", "8080"),
		AirflowExposePort:     newCfg("airflow.expose_port", "false"),
		ShowWarnings:          newCfg("show_warnings", "true"),
		Verbosity:             newCfg("verbosity", "warning"),
		HoustonDialTimeout:    newCfg("houston.dial_timeout", "10"),
		HoustonSkipVerifyTLS:  newCfg("houston.skip_verify_tls", "false"),
		DuplicateImageVolumes: newCfg("duplicate_volumes", "true"),
		SkipParse:             newCfg("skip_parse", "false"),
		Interactive:           newCfg("interactive", "false"),
		PageSize:              newCfg("page_size", "20"),
		UpgradeMessage:        newCfg("upgrade_message", "true"),
		DisableAstroRun:       newCfg("disable_astro_run", "false"),
		DisableEnvObjects:     newCfg("disable_env_objects", "false"),
	}

	// viperHome is the viper object in the users home directory
	viperHome *viper.Viper
	// viperProject is the viper object in a project directory
	viperProject *viper.Viper
	// createConfigPath dir path, file path
	dirPerm  os.FileMode = 0o775
	filePerm os.FileMode = 0o600
)

// InitConfig initializes the config files
func InitConfig(fs afero.Fs) {
	initHome(fs)
	initProject(fs)
}

// Init viper for config file in home directory
func initHome(fs afero.Fs) {
	viperHome = viper.New()
	viperHome.SetFs(fs)
	viperHome.SetConfigName(ConfigFileName)
	viperHome.SetConfigType(ConfigFileType)

	configPath := os.Getenv("ASTRO_HOME")
	if configPath != "" {
		HomeConfigPath = filepath.Join(configPath, ConfigDir)
		HomeConfigFile = filepath.Join(HomeConfigPath, ConfigFileNameWithExt)
	} else {
		HomeConfigPath = filepath.Join(HomePath, ConfigDir)
		HomeConfigFile = filepath.Join(HomeConfigPath, ConfigFileNameWithExt)
	}
	viperHome.SetConfigFile(HomeConfigFile)

	for _, cfg := range CFGStrMap {
		if len(cfg.Default) > 0 {
			viperHome.SetDefault(cfg.Path, cfg.Default)
		}
	}

	// If home config does not exist, create it
	homeConfigExists, _ := fileutil.Exists(HomeConfigFile, fs)
	if !homeConfigExists {
		err := CreateConfig(viperHome, HomeConfigPath, HomeConfigFile)
		if err != nil {
			fmt.Printf(configCreateHomeErrorMsg, err)
			return
		}
	}

	// Read in home config
	err := viperHome.ReadInConfig()
	if err != nil {
		fmt.Printf(configReadErrorMsg, err)
		return
	}
}

// Init viper for config file in project directory
// If project config does not exist, just exit
func initProject(fs afero.Fs) {
	// Set up viper object for project config
	viperProject = viper.New()
	viperProject.SetFs(fs)
	viperProject.SetConfigName(ConfigFileName)
	viperProject.SetConfigType(ConfigFileType)

	// Construct the path to the config file
	workingConfigPath := filepath.Join(WorkingPath, ConfigDir)

	workingConfigFile := filepath.Join(workingConfigPath, ConfigFileNameWithExt)

	// If path is empty or config file does not exist, just return
	workingConfigExists, _ := fileutil.Exists(workingConfigFile, fs)
	if workingConfigPath == "" || workingConfigPath == HomeConfigPath || !workingConfigExists {
		return
	}

	// Add the path we discovered
	viperProject.SetConfigFile(workingConfigFile)

	// Read in project config
	readErr := viperProject.ReadInConfig()
	if readErr != nil {
		fmt.Printf(configReadErrorMsg, readErr)
	}
}

// CreateProjectConfig creates a project config file
func CreateProjectConfig(projectPath string) {
	projectConfigDir := filepath.Join(projectPath, ConfigDir)
	projectConfigFile := filepath.Join(projectConfigDir, ConfigFileNameWithExt)

	err := CreateConfig(viperProject, projectConfigDir, projectConfigFile)
	if err != nil {
		fmt.Printf(configCreateHomeErrorMsg, err)
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
	err := os.MkdirAll(path, dirPerm)
	if err != nil {
		return fmt.Errorf("error creating config directory: %w", err)
	}

	_, err = os.Create(file)
	if err != nil {
		return fmt.Errorf("error creating config file: %w", err)
	}
	err = os.Chmod(file, filePerm)
	if err != nil {
		return fmt.Errorf("error creating config file: %w", err)
	}

	return saveConfig(v, file)
}

// ProjectConfigExists returns a boolean indicating if a project config file exists
func ProjectConfigExists() bool {
	return configExists(viperProject)
}

// IsProjectDir returns a boolean depending on if path is a valid project dir
func IsProjectDir(path string) (bool, error) {
	configPath := filepath.Join(path, ConfigDir)
	configFile := filepath.Join(configPath, ConfigFileNameWithExt)

	// Home directory is not a project directory
	if HomePath == path {
		return false, nil
	}

	return fileutil.Exists(configFile, nil)
}

// saveConfig will save the config to a file
func saveConfig(v *viper.Viper, file string) error {
	err := v.WriteConfigAs(file)
	if err != nil {
		return fmt.Errorf("error saving config: %w", err)
	}
	return nil
}
