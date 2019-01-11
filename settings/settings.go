package settings

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

var (
	// ConfigFileName is the name of the config files (home / project)
	ConfigFileName = "settings"
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
)

// ConfigSettings is the main builder of the settings package
func ConfigSettings(id string) {
	InitSettings()
	AddVariables(id)
	AddConnections(id)
	AddPools(id)
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
	// fmt.Println(viperSettings.Get("airflow"))
	if readErr != nil {
		fmt.Printf(messages.CONFIG_READ_ERROR, readErr)
	}
	err := viperSettings.Unmarshal(&settings)
	if err != nil {
		errors.Wrap(err, "unable to decode into struct")
	}
}

// AddVariables is a function to add Variables from settings.yaml
func AddVariables(id string) (string, error) {
	variables := settings.Airflow.Variables

	for _, variable := range variables {
		if len(variable.VariableName) == 0 && len(variable.VariableValue) > 0 {
			fmt.Print("Skipping Variable Creation: No Variable Name Specified.")

		} else {
			airflowCommand := fmt.Sprintf("airflow variables -s \"%s\" \"%s\"", variable.VariableName, variable.VariableValue)
			out, err := AirflowCommand(id, airflowCommand)
			if err != nil {
				return "", err
			}
			fmt.Printf("Added Variable: %s\n", variable.VariableName)
			return out, nil

		}
	}
	return "", nil
}

// AddConnections is a function to add Connections from settings.yaml
func AddConnections(id string) (string, error) {
	connections := settings.Airflow.Connections
	airflowCommand := fmt.Sprintf("airflow connections -l")
	out, err := AirflowCommand(id, airflowCommand)
	if err != nil {
		return "", err
	}
	for _, conn := range connections {
		if len(conn.ConnID) > 0 && len(conn.ConnType) == 0 && len(conn.ConnURI) == 0 {
			fmt.Printf("Skipping %s: ConnType or ConnUri must be specified.", conn.ConnID)
		} else {
			quotedConnID := "'" + conn.ConnID + "'"
			if strings.Contains(out, quotedConnID) {
				fmt.Printf("Found Connection: \"%s\"...replacing...\n", conn.ConnID)
				airflowCommand = fmt.Sprintf("airflow connections -d --conn_id \"%s\"", conn.ConnID)
				_, err = AirflowCommand(id, airflowCommand)
				if err != nil {
					return "", err
				}
			}
			airflowCommand = fmt.Sprintf("airflow connections -a --conn_id \"%s\" --conn_type \"%s\" --conn_uri \"%s\" --conn_extra \"%s\" --conn_host  \"%s\" --conn_login \"%s\" --conn_password \"%s\" --conn_schema \"%s\" --conn_port \"%v\"", conn.ConnID, conn.ConnType, conn.ConnURI, conn.ConnExtra, conn.ConnHost, conn.ConnLogin, conn.ConnPassword, conn.ConnSchema, conn.ConnPort)
			_, err = AirflowCommand(id, airflowCommand)
			if err != nil {
				return "", err
			}
			fmt.Printf("Added Connection: %s\n", conn.ConnID)
		}
	}
	return "", nil
}

// AddPools  is a function to add Pools from settings.yaml
func AddPools(id string) (string, error) {
	pools := settings.Airflow.Pools
	for _, pool := range pools {
		if len(pool.PoolName) == 0 && pool.PoolSlot > 0 {
			fmt.Print("Skipping Pool Creation: No Pool Name Specified.")
		} else {
			airflowCommand := fmt.Sprintf("airflow pool -s \"%s\" \"%v\" \"%s\"", pool.PoolName, pool.PoolSlot, pool.PoolDescription)
			_, err := AirflowCommand(id, airflowCommand)
			if err != nil {
				return "", err
			}
			fmt.Printf("Added Pool: %s\n", pool.PoolName)
		}
	}
	return "", nil
}

// AirflowCommand is the main method of interaction with Airflow
func AirflowCommand(id string, airflowCommand string) (string, error) {
	cmd := exec.Command("docker", "exec", "-it", id, "bash", "-c", airflowCommand)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		log.Fatal(err)
	}

	stringOut := string(out)

	return stringOut, err
}
