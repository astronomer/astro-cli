package settings

import (
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v2"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/docker"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

var (
	// ConfigFileName is the name of the config files (home / project)
	// ConfigFileName = "airflow_settings"
	// ConfigFileType is the config file extension
	ConfigFileType = "yaml"
	tmpFile        = "tmp.json"
	// WorkingPath is the path to the working directory
	WorkingPath, _ = fileutil.GetWorkingDir()

	// viperSettings is the viper object in a project directory
	viperSettings *viper.Viper

	settings Config

	// AirflowVersionTwo 2.0.0
	AirflowVersionTwo uint64 = 2

	// Monkey patched as of now to write unit tests
	// TODO: do replace this with interface based mocking once changes are in place in `airflow` package
	execAirflowCommand = docker.AirflowCommand
)

const (
	configReadErrorMsg = "Error reading config in home dir: %s\n"
	noColorString      = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"
)

var errNoID = errors.New("container ID is not found, the webserver may not be running")
var re = regexp.MustCompile(noColorString)

// ConfigSettings is the main builder of the settings package
func ConfigSettings(id, settingsFile string, version uint64, connections, variables, pools, logs bool) error {
	if id == "" {
		return errNoID
	}
	err := InitSettings(settingsFile)
	if err != nil {
		return err
	}
	if pools {
		AddPools(id, version, logs)
	}
	if variables {
		AddVariables(id, version, logs)
	}
	if connections {
		AddConnections(id, version, logs)
	}
	return nil
}

// InitSettings initializes settings file
func InitSettings(settingsFile string) error {
	// Set up viper object for project config
	viperSettings = viper.New()
	ConfigFileName := strings.Split(settingsFile, ".")[0]
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
	// Try and use old settings file if error
	if err != nil {
		return errors.Wrap(err, "unable to decode file")
	}
	return nil
}

// AddVariables is a function to add Variables from settings.yaml
func AddVariables(id string, version uint64, logs bool) {
	variables := settings.Airflow.Variables
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
			if logs {
				fmt.Println("Adding variable logs:\n" + out)
			}
			fmt.Printf("Added Variable: %s\n", variable.Variable_Name)
		}
	}
}

// AddConnections is a function to add Connections from settings.yaml
func AddConnections(id string, version uint64, logs bool) {
	connections := settings.Airflow.Connections
	baseCmd := "airflow connections "
	var baseAddCmd, baseRmCmd, baseListCmd, connIDArg, connTypeArg, connURIArg, connExtraArg, connHostArg, connLoginArg, connPasswordArg, connSchemaArg, connPortArg string
	if version >= AirflowVersionTwo {
		// Airflow 2.0.0 command
		// based on https://airflow.apache.org/docs/apache-airflow/2.0.0/cli-and-env-variables-ref.html
		baseAddCmd = baseCmd + "add "
		baseRmCmd = baseCmd + "delete "
		baseListCmd = baseCmd + "list -o plain"
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
		var j int
		conn := connections[i]
		if !objectValidator(0, conn.Conn_ID) {
			continue
		}

		var extra_string string
		v, ok := conn.Conn_Extra.(map[interface{}]interface{})
		if !ok {
			t, ok := conn.Conn_Extra.(string)
			if ok {
				extra_string = t
			}
		} else {
			extra_string = jsonString(v)
		}

		quotedConnID := "'" + conn.Conn_ID + "'"

		if strings.Contains(out, quotedConnID) || strings.Contains(out, conn.Conn_ID) {
			fmt.Printf("Updating Connection %q...\n", conn.Conn_ID)
			airflowCommand = fmt.Sprintf("%s %s %q", baseRmCmd, connIDArg, conn.Conn_ID)
			execAirflowCommand(id, airflowCommand)
		}

		if !objectValidator(1, conn.Conn_Type, conn.Conn_URI) {
			fmt.Printf("Skipping %s: conn_type or conn_uri must be specified.\n", conn.Conn_ID)
			continue
		}

		airflowCommand = fmt.Sprintf("%s %s '%s' ", baseAddCmd, connIDArg, conn.Conn_ID)
		if objectValidator(0, conn.Conn_Type) {
			airflowCommand += fmt.Sprintf("%s '%s' ", connTypeArg, conn.Conn_Type)
			j++
		}
		if extra_string != "" {
			airflowCommand += fmt.Sprintf("%s '%s' ", connExtraArg, extra_string)
		}
		if objectValidator(0, conn.Conn_Host) {
			airflowCommand += fmt.Sprintf("%s '%s' ", connHostArg, conn.Conn_Host)
			j++
		}
		if objectValidator(0, conn.Conn_Login) {
			airflowCommand += fmt.Sprintf("%s '%s' ", connLoginArg, conn.Conn_Login)
			j++
		}
		if objectValidator(0, conn.Conn_Password) {
			airflowCommand += fmt.Sprintf("%s '%s' ", connPasswordArg, conn.Conn_Password)
			j++
		}
		if objectValidator(0, conn.Conn_Schema) {
			airflowCommand += fmt.Sprintf("%s '%s' ", connSchemaArg, conn.Conn_Schema)
			j++
		}
		if conn.Conn_Port != 0 {
			airflowCommand += fmt.Sprintf("%s %v", connPortArg, conn.Conn_Port)
			j++
		}
		if objectValidator(0, conn.Conn_URI) && j == 0 {
			airflowCommand += fmt.Sprintf("%s '%s' ", connURIArg, conn.Conn_URI)
		}

		out := execAirflowCommand(id, airflowCommand)
		if logs {
			fmt.Println("Adding Connection logs:\n" + out)
		}
		fmt.Printf("Added Connection: %s\n", conn.Conn_ID)
	}
}

// AddPools  is a function to add Pools from settings.yaml
func AddPools(id string, version uint64, logs bool) {
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
				if logs {
					fmt.Println("Adding pool logs:\n" + out)
				}
				fmt.Printf("Added Pool: %s\n", pool.Pool_Name)
			} else {
				fmt.Printf("Skipping %s: Pool Slot must be set.\n", pool.Pool_Name)
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

func EnvExport(id, envFile string, version uint64, connections, variables, logs bool) error {
	if id == "" {
		return errNoID
	}
	var parseErr bool
	if version >= AirflowVersionTwo {
		// env export variables if variables is true
		if variables {
			err := EnvExportVariables(id, envFile, logs)
			if err != nil {
				fmt.Println(err)
				parseErr = true
			}
		}
		// env export connections if connections is true
		if connections {
			err := EnvExportConnections(id, envFile, logs)
			if err != nil {
				fmt.Println(err)
				parseErr = true
			}
		}
		if parseErr {
			return errors.New("there was an error during env export")
		}
		return nil
	}

	return errors.New("Command must be used with Airflow 2.X")
}

func EnvExportVariables(id, envFile string, logs bool) error {
	// setup airflow command to export variables
	airflowCommand := "airflow variables export tmp.var"
	out := execAirflowCommand(id, airflowCommand)
	if logs {
		fmt.Println("Env Export Variables logs:\n" + out)
	}

	if strings.Contains(out, "successfully") {
		// get variables from file created by airflow command
		fileCmd := "cat tmp.var"
		out = execAirflowCommand(id, fileCmd)

		m := map[string]string{}
		err := json.Unmarshal([]byte(out), &m)
		if err != nil {
			fmt.Println(err)
			fmt.Println("variable json decode unsuccessful")
		}
		// add variables to the env file
		f, err := os.OpenFile(envFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gomnd
		if err != nil {
			return errors.Wrap(err, "Writing variables to file unsuccessful")
		}

		defer f.Close()

		for k, v := range m {
			fmt.Println("Exporting Variable: " + k)
			_, err := f.WriteString("\nAIRFLOW_VAR_" + strings.ToUpper(k) + "=" + v)
			if err != nil {
				fmt.Println(err)
				fmt.Printf("error adding variable %s to file\n", k)
			}
		}
		fmt.Println("Aiflow variables successfully export to the file " + envFile + "\n")
		rmCmd := "rm tmp.var"
		_ = execAirflowCommand(id, rmCmd)
		return nil
	}
	return errors.New("variable export unsuccessful")
}

func EnvExportConnections(id, envFile string, logs bool) error {
	// Airflow command to export connections to env uris
	airflowCommand := "airflow connections export tmp.connections --file-format env"
	out := execAirflowCommand(id, airflowCommand)
	if logs {
		fmt.Println("Env Export Connections logs:\n" + out)
	}

	if strings.Contains(out, "successfully") {
		// get connections from file craeted by airflow command
		fileCmd := "cat tmp.connections"
		out = execAirflowCommand(id, fileCmd)

		vars := strings.Split(out, "\n")
		// add connections to the env file
		f, err := os.OpenFile(envFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gomnd
		if err != nil {
			return errors.Wrap(err, "Writing connections to file unsuccessful")
		}

		defer f.Close()

		for i := range vars {
			varSplit := strings.SplitN(vars[i], "=", 2)
			if len(varSplit) > 1 {
				fmt.Println("Exporting Connection: " + varSplit[0])
				_, err := f.WriteString("\nAIRFLOW_CONN_" + strings.ToUpper(varSplit[0]) + "=" + varSplit[1])
				if err != nil {
					fmt.Println(err)
					fmt.Printf("error adding connection %s to file\n", varSplit[0])
				}
			}
		}
		fmt.Println("Aiflow connections successfully export to the file " + envFile + "\n")
		rmCmd := "rm tmp.connection"
		_ = execAirflowCommand(id, rmCmd)
		return nil
	}
	return errors.New("connection export unsuccessful")
}

func Export(id, settingsFile string, version uint64, connections, variables, pools, logs bool) error {
	if id == "" {
		return errNoID
	}
	// init settings file
	err := InitSettings(settingsFile)
	if err != nil {
		return err
	}
	var parseErr bool
	// export Airflow Objects
	if version >= AirflowVersionTwo {
		if pools {
			err = ExportPools(id, logs)
			if err != nil {
				fmt.Println(err)
				parseErr = true
			}
		}
		if variables {
			err = ExportVariables(id, logs)
			if err != nil {
				fmt.Println(err)
				parseErr = true
			}
		}
		if connections {
			err := ExportConnections(id, logs)
			if err != nil {
				fmt.Println(err)
				parseErr = true
			}
		}
		if parseErr {
			return errors.New("there was an error during export")
		}
		return nil
	}

	return errors.New("Command must be used with Airflow 2.X")
}

func ExportConnections(id string, logs bool) error {
	// Setup airflow command to export connections
	airflowCommand := "airflow connections list -o yaml"
	out := execAirflowCommand(id, airflowCommand)
	if logs {
		fmt.Println("Export Connections logs:\n" + out)
	}
	// remove all color from output of the airflow command
	plainOut := re.ReplaceAllString(out, "")
	// remove extra warning text
	yamlCons := "- conn_id:" + strings.SplitN(plainOut, "- conn_id:", 2)[1]

	var connections ListConnections

	err := yaml.Unmarshal([]byte(yamlCons), &connections)
	if err != nil {
		return err
	}
	// add connections to settings file
	for i := range connections {
		port, err := strconv.Atoi(connections[i].ConnPort)
		if err != nil {
			fmt.Println("Issue with parsing port number: ")
			fmt.Println(err)
		}
		fmt.Println("Exporting Connection: " + connections[i].ConnID)

		newConnection := Connection{
			Conn_ID:       connections[i].ConnID,
			Conn_Type:     connections[i].ConnType,
			Conn_Host:     connections[i].ConnHost,
			Conn_Schema:   connections[i].ConnSchema,
			Conn_Login:    connections[i].ConnLogin,
			Conn_Password: connections[i].ConnPassword,
			Conn_Port:     port,
			Conn_URI:      connections[i].ConnURI,
			Conn_Extra:    connections[i].ConnExtra,
		}

		settings.Airflow.Connections = append(settings.Airflow.Connections, newConnection)
	}
	// write to settings file
	viperSettings.Set("airflow", settings.Airflow)
	err = viperSettings.WriteConfig()
	if err != nil {
		return err
	}
	fmt.Println("successfully exported Connections\n")
	return nil
}

func ExportVariables(id string, logs bool) error {
	// setup files
	airflowCommand := "airflow variables export tmp.var"
	out := execAirflowCommand(id, airflowCommand)
	if logs {
		fmt.Println("Export Variables logs:\n" + out)
	}

	if strings.Contains(out, "successfully") {
		// get variables created by the airflow command
		fileCmd := "cat tmp.var"
		out = execAirflowCommand(id, fileCmd)

		m := map[string]string{}
		err := json.Unmarshal([]byte(out), &m)
		if err != nil {
			fmt.Println("variable json decode unsuccessful")
		}
		// add the variables to settings object
		for k, v := range m {
			newVariables := Variables{{k, v}}
			fmt.Println("Exporting Variable: " + k)
			settings.Airflow.Variables = append(settings.Airflow.Variables, newVariables...)
		}
		// write variables to settings file
		viperSettings.Set("airflow", settings.Airflow)
		err = viperSettings.WriteConfig()
		if err != nil {
			return err
		}
		rmCmd := "rm tmp.var"
		_ = execAirflowCommand(id, rmCmd)
		fmt.Println("successfully exported variables\n")
		return nil
	}
	return errors.New("variable export unsuccessful")
}

func ExportPools(id string, logs bool) error {
	// Setup airflow command to export pools
	airflowCommand := "airflow pools list -o yaml"
	out := execAirflowCommand(id, airflowCommand)
	if logs {
		fmt.Println("Export Pools logs:\n" + out)
	}

	// remove all color from output of the airflow command
	plainOut := re.ReplaceAllString(out, "")

	var pools ListPools
	// remove warnings and extra text from the the output
	yamlpools := "- description:" + strings.SplitN(plainOut, "- description:", 2)[1]

	err := yaml.Unmarshal([]byte(yamlpools), &pools)
	if err != nil {
		return err
	}
	// add pools to the settings object
	for i := range pools {
		if pools[i].PoolName != "default_pool" {
			slot, err := strconv.Atoi(pools[i].PoolSlot)
			if err != nil {
				fmt.Println("Issue with parsing pool slot number: ")
				fmt.Println(err)
			}
			fmt.Println("Exporting Pool: " + pools[i].PoolName)
			newPools := Pools{{pools[i].PoolName, slot, pools[i].PoolDescription}}
			settings.Airflow.Pools = append(settings.Airflow.Pools, newPools...)
		}
	}
	// write pools to the airflow settings file
	viperSettings.Set("airflow", settings.Airflow)
	err = viperSettings.WriteConfig()
	if err != nil {
		return err
	}
	fmt.Println("successfully exported pools\n")
	return nil
}

func jsonString(connExtra map[interface{}]interface{}) string {
	var extra_string string
	i := 0
	for k, v := range connExtra {
		key, ok := k.(string)
		value, ok := v.(string)
		if ok {
			if i == 0 {
				extra_string = extra_string + "\"" + key + "\": \"" + value + "\""
			} else {
				extra_string = extra_string + ", \"" + key + "\": \"" + value + "\""
			}
			i++
		}
	}
	if extra_string != "" {
		extra_string = "{" + extra_string + "}"
	}
	return extra_string
}
