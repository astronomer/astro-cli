package settings

import (
	"fmt"
	"strings"
)

// AddVariablesoOld is a function to add Variables from settings.yaml
func AddVariablesOld(id string, version uint64) {
	variables := oldSettings.OldAirflow.Variables
	for _, variable := range variables {
		if !objectValidator(0, variable.Variable_Name) {
			if objectValidator(0, variable.Variable_Value) {
				fmt.Print("Skipping Variable Creation: No Variable Name Specified.\n")
			}
		} else if objectValidator(0, variable.Variable_Value) {
			baseCmd := "airflow variables "
			if version >= AirflowVersionTwo {
				baseCmd += "set %s " // Airflow 2.0.0 command
			} else {
				baseCmd += "-s %s"
			}

			airflowCommand := fmt.Sprintf(baseCmd, variable.Variable_Name)

			airflowCommand += fmt.Sprintf("'%s'", variable.Variable_Value)
			out := execAirflowCommand(id, airflowCommand)
			fmt.Println("Adding variable logs:\n" + out)
			fmt.Printf("Added Variable: %s\n", variable.Variable_Name)
		}
	}
}

// AddConnections is a function to add Connections from settings.yaml
func AddConnectionsOld(id string, version uint64) {
	connections := oldSettings.OldAirflow.OldConnections
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
		fmt.Println(airflowCommand)
		out := execAirflowCommand(id, airflowCommand)
		fmt.Println("Adding connection logs:\n" + out)
		fmt.Printf("Added Connection: %s\n", conn.ConnID)
	}
}

// AddPoolsOld  is a function to add Pools from settings.yaml
func AddPoolsOld(id string, version uint64) {
	pools := oldSettings.OldAirflow.Pools
	baseCmd := "airflow "

	if version >= AirflowVersionTwo {
		// Airflow 2.0.0 command
		// based on https://airflow.apache.org/docs/apache-airflow/2.0.0/cli-and-env-variables-ref.html
		baseCmd += "pools set "
	} else {
		baseCmd += "pool -s "
	}

	for _, pool := range pools {
		if objectValidator(0, pool.Pool_Name) {
			airflowCommand := fmt.Sprintf("%s %s ", baseCmd, pool.Pool_Name)
			if pool.Pool_Slot != 0 {
				airflowCommand += fmt.Sprintf("%v ", pool.Pool_Slot)
				if objectValidator(0, pool.Pool_Description) {
					airflowCommand += fmt.Sprintf("'%s' ", pool.Pool_Description)
				} else {
					airflowCommand += "''"
				}
				fmt.Println(airflowCommand)
				out := execAirflowCommand(id, airflowCommand)
				fmt.Println("Adding pool logs:\n" + out)
				fmt.Printf("Added Pool: %s\n", pool.Pool_Name)
			} else {
				fmt.Printf("Skipping %s: Pool Slot must be set.\n", pool.Pool_Name)
			}
		}
	}
}
