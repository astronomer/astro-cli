package settings

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/fileutil"

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

	// AirflowVersionTwo is for Airflow Version 2.0.0
	AirflowVersionTwo uint64 = 2
)

// ConfigSettings is the main builder of the settings package
func ConfigSettings(containerEngine airflow.ContainerHandler, id string, version uint64) {
	InitSettings()
	AddConnections(containerEngine, id, version)
	AddPools(containerEngine, id, version)
	AddVariables(containerEngine, id, version)
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
		fmt.Printf(messages.ErrReadingConfig, readErr)
	}

	err := viperSettings.Unmarshal(&settings)
	if err != nil {
		fmt.Println("Unable to decode settings struct: ", err.Error())
	}
}

// AddVariables is a function to add Variables from settings.yaml
func AddVariables(containerEngine airflow.ContainerHandler, id string, version uint64) {
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
				baseCmd += "-s %s " // Airflow 1.0.0 command
			}

			airflowCommand := fmt.Sprintf(baseCmd, variable.VariableName)

			airflowCommand += fmt.Sprintf("'%s'", variable.VariableValue)

			_, err := containerEngine.ExecCommand(id, airflowCommand)
			if err != nil {
				fmt.Printf("Error adding variable %s: %s\n", variable.VariableName, err.Error())
			} else {
				fmt.Printf("Added Variable: %s\n", variable.VariableName)
			}
		}
	}
}

// AddConnections is a function to add Connections from settings.yaml
func AddConnections(containerEngine airflow.ContainerHandler, id string, airflowVersion uint64) {
	connections := settings.Airflow.Connections
	baseCmd := "airflow connections "
	var baseAddCmd, baseRmCmd, baseListCmd, connIDArg, connTypeArg, connURIArg, connExtraArg, connHostArg, connLoginArg, connPasswordArg, connSchemaArg, connPortArg string
	if airflowVersion >= AirflowVersionTwo {
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
	out, _ := containerEngine.ExecCommand(id, airflowCommand)
	for idx := range connections {
		conn := connections[idx]
		if !objectValidator(0, conn.ConnID) {
			continue
		}
		quotedConnID := "'" + conn.ConnID + "'"

		if strings.Contains(out, quotedConnID) {
			fmt.Printf("Found Connection: \"%s\"...replacing...\n", conn.ConnID)
			airflowCommand = fmt.Sprintf("%s %s \"%s\"", baseRmCmd, connIDArg, conn.ConnID)
			_, err := containerEngine.ExecCommand(id, airflowCommand)
			if err != nil {
				fmt.Printf("Error replacing connection %s: %s\n", conn.ConnID, err.Error())
			}
		}

		if !objectValidator(1, conn.ConnType, conn.ConnURI) {
			fmt.Printf("Skipping %s: conn_type or conn_uri must be specified.\n", conn.ConnID)
			continue
		}
		airflowCommand = prepareAddConnCmd(baseAddCmd, connIDArg, connTypeArg, connURIArg, connExtraArg, connHostArg, connLoginArg, connPasswordArg, connSchemaArg, connPortArg, &conn)
		_, err := containerEngine.ExecCommand(id, airflowCommand)
		if err != nil {
			fmt.Printf("Error adding connection %s: %s\n", conn.ConnID, err.Error())
		} else {
			fmt.Printf("Added Connection: %s\n", conn.ConnID)
		}
	}
}

// AddPools  is a function to add Pools from settings.yaml
func AddPools(containerEngine airflow.ContainerHandler, id string, airflowVersion uint64) {
	pools := settings.Airflow.Pools
	baseCmd := "airflow "
	if airflowVersion >= AirflowVersionTwo {
		// Airflow 2.0.0 command

		// based on https://airflow.apache.org/docs/apache-airflow/2.0.0/cli-and-env-variables-ref.html
		baseCmd += "pools set "
	} else {
		// Airflow 1.0.0 command
		// based on https://airflow.apache.org/docs/apache-airflow/1.10.12/usage-cli.html
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
					airflowCommand += ""
				}
				_, err := containerEngine.ExecCommand(id, airflowCommand)
				if err != nil {
					fmt.Printf("Error adding pool %s: %s\n", pool.PoolName, err.Error())
				} else {
					fmt.Printf("Added Pool: %s\n", pool.PoolName)
				}
			} else {
				fmt.Printf("Skipping %s: Pool Slot must be set.\n", pool.PoolName)
			}
		}
	}
}

func prepareAddConnCmd(baseAddCmd, connIDArg, connTypeArg, connURIArg, connExtraArg, connHostArg, connLoginArg, connPasswordArg, connSchemaArg, connPortArg string, conn *Connection) string {
	if conn == nil {
		return ""
	}
	airflowCommand := fmt.Sprintf("%s %s \"%s\" ", baseAddCmd, connIDArg, conn.ConnID)
	if objectValidator(0, conn.ConnType) {
		airflowCommand += fmt.Sprintf("%s \"%s\" ", connTypeArg, conn.ConnType)
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
	return airflowCommand
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
