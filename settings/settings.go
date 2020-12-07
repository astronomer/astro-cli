package settings

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/docker"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

var (
	// ConfigFileName is the name of the config files (home / project)
	ConfigFileName = "airflow_settings"
	// ConfigFileType is the config file extension
	ConfigFileType = "yaml"
	// ConfigFileNameWithExt is the config filename with extension
	ConfigFileNameWithExt = fmt.Sprintf("%s.%s", ConfigFileName, ConfigFileType)
	// HomePath is the path to a users home directory
	HomePath, _ = fileutil.GetHomeDir()
	// HomeConfigFile is the global config file
	HomeConfigFile = filepath.Join(HomePath, ConfigFileNameWithExt)
	// WorkingPath is the path to the working directory
	WorkingPath, _ = fileutil.GetWorkingDir()

	// viperSettings is the viper object in a project directory
	viperSettings *viper.Viper

	settings Config

	// Version 2.0.0
	NewAirflowVersion uint64 = 2
)

// ConfigSettings is the main builder of the settings package
func ConfigSettings(id string, version uint64) {
	InitSettings()
	AddConnections(id)
	AddPools(id)
	AddVariables(id, version)
}

// InitSettings initializes settings file
func InitSettings() {
	// Set up viper object for project config
	viperSettings = viper.New()
	viperSettings.SetConfigName(ConfigFileName)
	viperSettings.SetConfigType(ConfigFileType)
	workingConfigFile := filepath.Join(WorkingPath, ConfigFileNameWithExt)
	// Add the path we discovered
	viperSettings.SetConfigFile(workingConfigFile)

	// Read in project config
	readErr := viperSettings.ReadInConfig()

	if readErr != nil {
		fmt.Printf(messages.CONFIG_READ_ERROR, readErr)
	}

	err := viperSettings.Unmarshal(&settings)

	if err != nil {
		errors.Wrap(err, "unable to decode into struct")
	}
}

// AddVariables is a function to add Variables from settings.yaml
func AddVariables(id string, version uint64) {
	variables := settings.Airflow.Variables

	for _, variable := range variables {
		if !objectValidator(0, variable.VariableName) {
			if objectValidator(0, variable.VariableValue) {
				fmt.Print("Skipping Variable Creation: No Variable Name Specified.\n")
			}
		} else {
			if objectValidator(0, variable.VariableValue) {

				baseCmd := "airflow variables "
				if version >= NewAirflowVersion {
					baseCmd += "set %s " // Airflow 2.0.0 command
				} else {
					baseCmd += "-s %s " // Airflow 1.0.0 command
				}

				airflowCommand := fmt.Sprintf(baseCmd, variable.VariableName)

				airflowCommand += fmt.Sprintf("'%s'", variable.VariableValue)

				docker.AirflowCommand(id, airflowCommand)
				fmt.Printf("Added Variable: %s\n", variable.VariableName)
			}
		}
	}
}

// AddConnections is a function to add Connections from settings.yaml
func AddConnections(id string) {
	connections := settings.Airflow.Connections
	airflowCommand := fmt.Sprintf("airflow connections -l")
	out := docker.AirflowCommand(id, airflowCommand)

	for _, conn := range connections {
		if objectValidator(0, conn.ConnID) {
			quotedConnID := "'" + conn.ConnID + "'"

			if strings.Contains(out, quotedConnID) {
				fmt.Printf("Found Connection: \"%s\"...replacing...\n", conn.ConnID)
				airflowCommand = fmt.Sprintf("airflow connections -d --conn_id \"%s\"", conn.ConnID)
				docker.AirflowCommand(id, airflowCommand)
			}

			if !objectValidator(1, conn.ConnType, conn.ConnURI) {
				fmt.Printf("Skipping %s: conn_type or conn_uri must be specified.\n", conn.ConnID)
			} else {
				airflowCommand = fmt.Sprintf("airflow connections -a --conn_id \"%s\" ", conn.ConnID)
				if objectValidator(0, conn.ConnType) {
					airflowCommand += fmt.Sprintf("--conn_type \"%s\" ", conn.ConnType)
				}
				if objectValidator(0, conn.ConnURI) {
					airflowCommand += fmt.Sprintf("--conn_uri '%s' ", conn.ConnURI)
				}
				if objectValidator(0, conn.ConnExtra) {
					airflowCommand += fmt.Sprintf("--conn_extra '%s' ", conn.ConnExtra)
				}
				if objectValidator(0, conn.ConnHost) {
					airflowCommand += fmt.Sprintf("--conn_host '%s' ", conn.ConnHost)
				}
				if objectValidator(0, conn.ConnLogin) {
					airflowCommand += fmt.Sprintf("--conn_login '%s' ", conn.ConnLogin)
				}
				if objectValidator(0, conn.ConnPassword) {
					airflowCommand += fmt.Sprintf("--conn_password '%s' ", conn.ConnPassword)
				}
				if objectValidator(0, conn.ConnSchema) {
					airflowCommand += fmt.Sprintf("--conn_schema '%s' ", conn.ConnSchema)
				}
				if conn.ConnPort != 0 {
					airflowCommand += fmt.Sprintf("--conn_port %v", conn.ConnPort)
				}

				docker.AirflowCommand(id, airflowCommand)
				fmt.Printf("Added Connection: %s\n", conn.ConnID)
			}
		}
	}
}

// AddPools  is a function to add Pools from settings.yaml
func AddPools(id string) {
	pools := settings.Airflow.Pools
	for _, pool := range pools {
		if objectValidator(0, pool.PoolName) {
			airflowCommand := fmt.Sprintf("airflow pool -s %s ", pool.PoolName)
			if pool.PoolSlot != 0 {
				airflowCommand += fmt.Sprintf("%v ", pool.PoolSlot)
				if objectValidator(0, pool.PoolDescription) {
					airflowCommand += fmt.Sprintf("'%s' ", pool.PoolDescription)
				} else {
					airflowCommand += fmt.Sprint("\"\"")
				}
				docker.AirflowCommand(id, airflowCommand)
				fmt.Printf("Added Pool: %s\n", pool.PoolName)
			} else {
				fmt.Printf("Skipping %s: Pool Slot must be set.\n", pool.PoolName)
			}
		}
	}
}

func objectValidator(bound int, args ...string) bool {
	count := 0
	for _, arg := range args {
		if len(arg) == 0 {
			count++
		}
	}
	if count > bound {
		return false
	}
	return true
}
