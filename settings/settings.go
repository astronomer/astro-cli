package settings

import (
	"fmt"
	"path/filepath"
	"strings"
	"encoding/json"
	"gopkg.in/yaml.v2"
	"regexp"
	"strconv"
	"os"

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
	// WorkingPath is the path to the working directory
	WorkingPath, _ = fileutil.GetWorkingDir()

	// viperSettings is the viper object in a project directory
	viperSettings *viper.Viper

	settings Config
	oldSettings OldConfig

	// AirflowVersionTwo 2.0.0
	AirflowVersionTwo uint64 = 2

	// Monkey patched as of now to write unit tests
	// TODO: do replace this with interface based mocking once changes are in place in `airflow` package
	execAirflowCommand = docker.AirflowCommand
	old bool
)

const configReadErrorMsg = "Error reading config in home dir: %s\n"
const ansi = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"
var re = regexp.MustCompile(ansi)

// ConfigSettings is the main builder of the settings package
func ConfigSettings(id, settingsFile string, version uint64, connections, variables, pools bool) error {
	err := InitSettings(settingsFile)
	if err != nil {
		return err
	}
	if old {
		if pools {
			AddPoolsOld(id, version)
		}
		if variables {
			AddVariablesOld(id, version)
		}
		if connections {
			AddConnectionsOld(id, version)
		}
		return nil
	}
	if pools {
		AddPools(id, version)
	}
	if variables {
		AddVariables(id, version)
	}
	if connections {
		AddConnections(id, version)
	}
	return nil
}

// InitSettings initializes settings file
func InitSettings(settingsFile string) error {
	// Set up viper object for project config
	viperSettings = viper.New()
	viperSettings.SetConfigName(settingsFile)
	viperSettings.SetConfigType(ConfigFileType)
	workingConfigFile := filepath.Join(WorkingPath, fmt.Sprintf("%s.%s", settingsFile, ConfigFileType))
	// Add the path we discovered
	viperSettings.SetConfigFile(workingConfigFile)

	// Read in project config
	readErr := viperSettings.ReadInConfig()

	if readErr != nil {
		fmt.Printf(configReadErrorMsg, readErr)
	}

	err := viperSettings.Unmarshal(&settings)
	if err != nil {
		err := viperSettings.Unmarshal(&oldSettings)
		return errors.Wrap(err, "unable to decode into struct")
		old = true
	}
	return nil
}

// AddVariables is a function to add Variables from settings.yaml
func AddVariables(id string, version uint64) {
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
			fmt.Println("Adding variable logs:\n" + out)
			fmt.Printf("Added Variable: %s\n", variable.Variable_Name)
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
		conn := connections[i]
		if !objectValidator(0, conn.Conn_ID) {
			continue
		}
	
		var extra_string string
		i = 0
		for k, v := range conn.Conn_Extra {
			if i == 0 {
				extra_string = extra_string + "\"" + k + "\": \"" + v + "\""
			} else {
				extra_string = extra_string + ", \"" + k + "\": \"" + v + "\""
			}
			i++
		}
		if extra_string != "" {
			extra_string = "{" + extra_string + "}"
		}

		quotedConnID := "'" + conn.Conn_ID + "'"

		if strings.Contains(out, quotedConnID) || strings.Contains(out, conn.Conn_ID)  {
			fmt.Printf("Found Connection: %q...replacing...\n", conn.Conn_ID)
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
		}
		if objectValidator(0, conn.Conn_URI) {
			airflowCommand += fmt.Sprintf("%s '%s' ", connURIArg, conn.Conn_URI)
		}
		if extra_string != "" {
			airflowCommand += fmt.Sprintf("%s '%s' ", connExtraArg, extra_string)
		}
		if objectValidator(0, conn.Conn_Host) {
			airflowCommand += fmt.Sprintf("%s '%s' ", connHostArg, conn.Conn_Host)
		}
		if objectValidator(0, conn.Conn_Login) {
			airflowCommand += fmt.Sprintf("%s '%s' ", connLoginArg, conn.Conn_Login)
		}
		if objectValidator(0, conn.Conn_Password) {
			airflowCommand += fmt.Sprintf("%s '%s' ", connPasswordArg, conn.Conn_Password)
		}
		if objectValidator(0, conn.Conn_Schema) {
			airflowCommand += fmt.Sprintf("%s '%s' ", connSchemaArg, conn.Conn_Schema)
		}
		if conn.Conn_Port != 0 {
			airflowCommand += fmt.Sprintf("%s %v", connPortArg, conn.Conn_Port)
		}
		out := execAirflowCommand(id, airflowCommand)
		fmt.Println("Adding connection logs:\n" + out)
		fmt.Printf("Added Connection: %s\n", conn.Conn_ID)
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

func objectValidator(bound int, args ...string) bool {
	count := 0
	for _, arg := range args {
		if arg == "" {
			count++
		}
	}
	return count <= bound
}

func SettingsEnvExport(id, envFile string, version uint64, connections, variables bool) error {
	if version >= AirflowVersionTwo {
		// env export variables if varaibles is true
		if variables {
			err := EnvExportVariables(id, envFile)
			if err != nil { 
				fmt.Println(err)
			}
		}
		// env export connections if connections is true
		if connections {
			err := EnvExportConnections(id, envFile)
			if err != nil { 
				fmt.Println(err)
			}
		}
		return nil
	}

	return errors.New("Command must be used with Airflow 2.X")
}

func EnvExportVariables(id, envFile string) error {
	// setup files
	tmpFile := "tmp.json"
	// setup airflow command to export variables
	airflowCommand := "airflow variables export " + tmpFile
	out := execAirflowCommand(id, airflowCommand)

	if strings.Contains(out, "successfully") {
		// get varaibles from file created by airflow command
		fileCmd := "cat " + tmpFile
		out = execAirflowCommand(id, fileCmd)

		m := map[string]string{}
		err := json.Unmarshal([]byte(out), &m)
		if err != nil {
			fmt.Println("variable json decode unsucessful")
		}
		// add variables to the env file
		f, err := os.OpenFile(envFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gomnd
		if err != nil {
			fmt.Println("Writing variables to file unsuccessful")
		}

		defer f.Close()

		for k, v := range m { 
			f.WriteString("\nAIRFLOW_VAR_" + strings.ToUpper(k) + "=" + v )
		}
		fmt.Println("Aiflow variables successfully export to the file " + envFile)
		rmCmd := "rm " + tmpFile 
		_ = execAirflowCommand(id, rmCmd)
		return nil
	}
	return errors.New("variable export unsucessful")
}

func EnvExportConnections(id, envFile string) error {
	// setup files
	tmpFile := "tmp.env"
	// Airflow command to export connections to env uris
	airflowCommand := "airflow connections export " + tmpFile +  " --file-format env"
	out := execAirflowCommand(id, airflowCommand)

	if strings.Contains(out, "successfully") {
		// get connections from file craeted by airflow command
		fileCmd := "cat " + tmpFile
		out = execAirflowCommand(id, fileCmd)

		vars := strings.Split(out, "\n")
		// add connections to the env file
		f, err := os.OpenFile(envFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gomnd
		if err != nil {
			fmt.Println("Writing connections to file unsuccessful")
		}

		defer f.Close()

		for i := range vars { 
			varSplit := strings.SplitN(vars[i], "=", 2)
			if len(varSplit) > 1 {
				f.WriteString("\nAIRFLOW_CONN_" + strings.ToUpper(varSplit[0]) + "=" + varSplit[1] )
			}
		}
		fmt.Println("Aiflow connections successfully export to the file " + envFile)
		rmCmd := "rm " + tmpFile 
		_ = execAirflowCommand(id, rmCmd)
		return nil
	}
	return errors.New("connection export unsucessful")
}

func SettingsExport(id, settingsFile string, version uint64, connections, variables, pools bool) error {
	// init settings file
	err := InitSettings(settingsFile)
	if err != nil {
		return err
	}
	// export Airflow Objects
	if version >= AirflowVersionTwo {
		if pools {
			err = ExportPools(id)
			if err != nil { 
				fmt.Println(err)
			}
		}
		if variables {
			err = ExportVariables(id)
			if err != nil { 
				fmt.Println(err)
			}
		}
		if connections {
			err := ExportConnections(id)
			if err != nil { 
				fmt.Println(err)
			}
		}
		return nil
	}

	return errors.New("Command must be used with Airflow 2.X")
}

func ExportConnections(id string) error {
	// Setup airflow command to export connections
	airflowCommand := "airflow connections list -o yaml"
	out := execAirflowCommand(id, airflowCommand)
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

		port, err :=  strconv.Atoi(connections[i].ConnPort)
		if err != nil {
			fmt.Println("Issue with parsing port number: ")
			fmt.Println(err)
		}

		newConnection := Connection{
			Conn_ID: connections[i].ConnID,
			Conn_Type: connections[i].ConnType,
			Conn_Host: connections[i].ConnHost,
			Conn_Schema: connections[i].ConnSchema,
			Conn_Login: connections[i].ConnLogin,
			Conn_Password: connections[i].ConnPassword,
			Conn_Port: port,
			Conn_URI: connections[i].ConnURI,
			Conn_Extra: connections[i].ConnExtra,
		}

		settings.Airflow.Connections = append(settings.Airflow.Connections, newConnection)
	}
	// write to settings file
	viperSettings.Set("airflow", settings.Airflow)
	viperSettings.WriteConfig()
	fmt.Println("successfully exported Connections")
	return nil
}

func ExportVariables(id string) error {
	// setup files
	tmpFile := "tmp.json"
	airflowCommand := "airflow variables export " + tmpFile
	out := execAirflowCommand(id, airflowCommand)

	if strings.Contains(out, "successfully") {
		// get variables created by the airflow command
		fileCmd := "cat " + tmpFile
		out = execAirflowCommand(id, fileCmd)

		m := map[string]string{}
		err := json.Unmarshal([]byte(out), &m)
		if err != nil {
			fmt.Println("variable json decode unsucessful")
		}
		// add the variables to settings object
		for k, v := range m {

			newVariables := Variables{{k, v}}

			for _, variable := range newVariables {
				settings.Airflow.Variables = append(settings.Airflow.Variables, variable)
			}
		}
		// write variables to settings file
		viperSettings.Set("airflow", settings.Airflow)
		viperSettings.WriteConfig()
		fmt.Println("successfully exported variables")
		return nil
	}
	return errors.New("variable export unsucessful")
}

func ExportPools(id string) error {
	// Setup airflow command to export pools
	airflowCommand := "airflow pools list -o yaml"
	out := execAirflowCommand(id, airflowCommand)
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
			slot, err :=  strconv.Atoi(pools[i].PoolSlot)
			if err != nil {
				fmt.Println("Issue with parsing pool slot number: ")
				fmt.Println(err)
			}

			newPools := Pools{{pools[i].PoolName, slot, pools[i].PoolDescription}}

			for _, pool := range newPools {
				settings.Airflow.Pools = append(settings.Airflow.Pools, pool)
			}
		}
	}
	// write pools to the airlfow settings file
	viperSettings.Set("airflow", settings.Airflow)
	viperSettings.WriteConfig()
	fmt.Println("successfully exported pools")
	return nil
}