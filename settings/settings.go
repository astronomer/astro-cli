package settings

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/docker"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

var (
	// ConfigFileName is the name of the config files (home / project)
	ConfigFileName = "airflow_settings"
	// ConfigFileType is the config file extension
	ConfigFileType = "yaml"
	// WorkingPath is the path to the working directory
	WorkingPath, _ = fileutil.GetWorkingDir()

	// viperSettings is the viper object in a project directory
	viperSettings *viper.Viper

	settings Config

	// Version 2.0.0
	AirflowVersionTwo uint64 = 2

	// Monkey patched as of now to write unit tests
	// TODO: do replace this with interface based mocking once changes are in place in `airflow` package
	execAirflowCommand = docker.AirflowCommand
)

const configReadErrorMsg = "Error reading config in home dir: %s\n"

// ConfigSettings is the main builder of the settings package
func ConfigSettings(id string, version uint64) error {
	err := InitSettings()
	if err != nil {
		return err
	}
	AddPools(id, version)
	AddVariables(id, version)
	AddConnections(id, version)
	return nil
}

// InitSettings initializes settings file
func InitSettings() error {
	// Set up viper object for project config
	viperSettings = viper.New()
	viperSettings.SetConfigName(ConfigFileName)
	viperSettings.SetConfigType(ConfigFileType)
	workingConfigFile := filepath.Join(WorkingPath, fmt.Sprintf("%s.%s", ConfigFileName, ConfigFileType))
	// Add the path we discovered
	viperSettings.SetConfigFile(workingConfigFile)

	// Read in project config
	readErr := viperSettings.ReadInConfig()

	if readErr != nil {
		fmt.Printf(configReadErrorMsg, readErr)
	}

	err := viperSettings.Unmarshal(&settings)
	if err != nil {
		return errors.Wrap(err, "unable to decode into struct")
	}
	return nil
}

// AddVariables is a function to add Variables from settings.yaml
func AddVariables(id string, version uint64) {
	variables := settings.Airflow.Variables
	for _, variable := range variables {
		if !objectValidator(0, variable.VariableName) {
			if objectValidator(0, variable.VariableValue) {
				fmt.Print("Skipping Variable Creation: No Variable Name Specified.\n")
			}
		} else if objectValidator(0, variable.VariableValue) {
			baseCmd := "airflow variables "
			if version >= AirflowVersionTwo {
				baseCmd += "set %s " // Airflow 2.0.0 command
			} else {
				baseCmd += "-s %s"
			}

			airflowCommand := fmt.Sprintf(baseCmd, variable.VariableName)

			airflowCommand += fmt.Sprintf("'%s'", variable.VariableValue)
			out := execAirflowCommand(id, airflowCommand)
			fmt.Println("Adding variable logs:\n" + out)
			fmt.Printf("Added Variable: %s\n", variable.VariableName)
		}
	}
}

// AddConnections is a function to add Connections from settings.yaml
func AddConnections(id string, version uint64) {
	connections := settings.Airflow.Connections
	baseCmd := "airflow connections "
	var baseAddCmd, baseRmCmd, baseListCmd, connIDArg, connTypeArg, connURIArg, connExtraArg, connHostArg, connLoginArg, connPasswordArg, connSchemaArg, connPortArg string
	if version >= AirflowVersionTwo {
		// Airflow 2.0.0 command
		// based on https://airflow.apache.org/docs/apache-airflow/2.0.0/cli-and-env-variables-ref.html
		baseAddCmd = baseCmd + "add "
		baseRmCmd = baseCmd + "delete "
		baseListCmd = baseCmd + "list "
		connIDArg = ""
		connTypeArg = "--conn-type"
		connURIArg = "--conn-uri"
		connExtraArg = "--conn-extra"
		connHostArg = "--conn-host"
		connLoginArg = "--conn-login"
		connPasswordArg = "--conn-password"
		connSchemaArg = "--conn-schema"
		connPortArg = "--conn-port"
	} else {
		// Airflow 1.0.0 command based on
		// https://airflow.readthedocs.io/en/1.10.12/cli-ref.html#connections
		baseAddCmd = baseCmd + "-a "
		baseRmCmd = baseCmd + "-d "
		baseListCmd = baseCmd + "-l "
		connIDArg = "--conn_id"
		connTypeArg = "--conn_type"
		connURIArg = "--conn_uri"
		connExtraArg = "--conn_extra"
		connHostArg = "--conn_host"
		connLoginArg = "--conn_login"
		connPasswordArg = "--conn_password"
		connSchemaArg = "--conn_schema"
		connPortArg = "--conn_port"
	}
	airflowCommand := baseListCmd
	out := execAirflowCommand(id, airflowCommand)

	for i := range connections {
		conn := connections[i]
		if !objectValidator(0, conn.ConnID) {
			continue
		}
		quotedConnID := "'" + conn.ConnID + "'"

		if strings.Contains(out, quotedConnID) {
			fmt.Printf("Found Connection: %q...replacing...\n", conn.ConnID)
			airflowCommand = fmt.Sprintf("%s %s %q", baseRmCmd, connIDArg, conn.ConnID)
			execAirflowCommand(id, airflowCommand)
		}

		if !objectValidator(1, conn.ConnType, conn.ConnURI) {
			fmt.Printf("Skipping %s: conn_type or conn_uri must be specified.\n", conn.ConnID)
			continue
		}

		airflowCommand = fmt.Sprintf("%s %s '%s' ", baseAddCmd, connIDArg, conn.ConnID)
		if objectValidator(0, conn.ConnType) {
			airflowCommand += fmt.Sprintf("%s '%s' ", connTypeArg, conn.ConnType)
		}
		if objectValidator(0, conn.ConnURI) {
			airflowCommand += fmt.Sprintf("%s '%s' ", connURIArg, conn.ConnURI)
		}
		if objectValidator(0, conn.ConnExtra) {
			airflowCommand += fmt.Sprintf("%s '%s' ", connExtraArg, conn.ConnExtra)
		}
		if objectValidator(0, conn.ConnHost) {
			airflowCommand += fmt.Sprintf("%s '%s' ", connHostArg, conn.ConnHost)
		}
		if objectValidator(0, conn.ConnLogin) {
			airflowCommand += fmt.Sprintf("%s '%s' ", connLoginArg, conn.ConnLogin)
		}
		if objectValidator(0, conn.ConnPassword) {
			airflowCommand += fmt.Sprintf("%s '%s' ", connPasswordArg, conn.ConnPassword)
		}
		if objectValidator(0, conn.ConnSchema) {
			airflowCommand += fmt.Sprintf("%s '%s' ", connSchemaArg, conn.ConnSchema)
		}
		if conn.ConnPort != 0 {
			airflowCommand += fmt.Sprintf("%s %v", connPortArg, conn.ConnPort)
		}
		out := execAirflowCommand(id, airflowCommand)
		fmt.Println("Adding connection logs:\n" + out)
		fmt.Printf("Added Connection: %s\n", conn.ConnID)
	}
}

// AddPools  is a function to add Pools from settings.yaml
func AddPools(id string, version uint64) {
	pools := settings.Airflow.Pools
	baseCmd := "airflow "

	if version >= AirflowVersionTwo {
		// Airflow 2.0.0 command
		// based on https://airflow.apache.org/docs/apache-airflow/2.0.0/cli-and-env-variables-ref.html
		baseCmd += "pools set "
	} else {
		baseCmd += "pool -s "
	}

	for _, pool := range pools {
		if objectValidator(0, pool.PoolName) {
			airflowCommand := fmt.Sprintf("%s %s ", baseCmd, pool.PoolName)
			if pool.PoolSlot != 0 {
				airflowCommand += fmt.Sprintf("%v ", pool.PoolSlot)
				if objectValidator(0, pool.PoolDescription) {
					airflowCommand += fmt.Sprintf("'%s' ", pool.PoolDescription)
				} else {
					airflowCommand += "''"
				}
				fmt.Println(airflowCommand)
				out := execAirflowCommand(id, airflowCommand)
				fmt.Println("Adding pool logs:\n" + out)
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
		if arg == "" {
			count++
		}
	}
	return count <= bound
}
