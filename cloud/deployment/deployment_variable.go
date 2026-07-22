package deployment

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var errVarCreateUpdate = errors.New(
	"there was an error while creating or updating one or more of the environment variables. Check the command output above for more information",
)

const maskedSecret = "****"

func VariableList(deploymentID, variableKey, ws, envFile, deploymentName string, useEnvFile bool, astroV1Client astrov1.APIClient, out io.Writer) error {
	environmentVariablesObjects := []astrov1.DeploymentEnvironmentVariable{}

	// get deployment
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, false, nil, astroV1Client)
	if err != nil {
		return err
	}

	if currentDeployment.EnvironmentVariables != nil {
		environmentVariablesObjects = *currentDeployment.EnvironmentVariables
	}
	if variableKey != "" {
		environmentVariablesObjects = slices.DeleteFunc(environmentVariablesObjects, func(v astrov1.DeploymentEnvironmentVariable) bool {
			return v.Key != variableKey
		})
	}

	// open env file
	if useEnvFile {
		err = writeVarToFile(environmentVariablesObjects, envFile)
		if err != nil {
			fmt.Fprintln(out, errors.Wrap(err, "unable to write environment variables to file"))
		}
	}
	if len(environmentVariablesObjects) == 0 {
		fmt.Fprintln(out, "\nNo variables found")
		return nil
	}

	makeVarTable(environmentVariablesObjects).Print(out) //nolint:errcheck // best-effort render to the terminal

	return nil
}

func makeVarTable(vars []astrov1.DeploymentEnvironmentVariable) *printutil.Table {
	table := printutil.Table{
		Padding:        []int{5, 30, 30, 50},
		DynamicPadding: true,
		Header:         []string{"#", "KEY", "VALUE", "SECRET"},
	}
	for i, variable := range vars {
		var value string

		if variable.IsSecret {
			value = maskedSecret
		} else if variable.Value != nil {
			value = *variable.Value
		}
		table.AddRow([]string{strconv.Itoa(i + 1), variable.Key, value, strconv.FormatBool(variable.IsSecret)}, false)
	}
	return &table
}

// this function modifies a deployment's environment variable object
// it is used to create and update deployment's environment variables
func VariableModify(
	deploymentID, variableKey, variableValue, ws, envFile, deploymentName string,
	variableList []string,
	useEnvFile, makeSecret, updateVars bool,
	astroV1Client astrov1.APIClient,
	out io.Writer,
) error {
	environmentVariablesObjects := []astrov1.DeploymentEnvironmentVariable{}

	// get deployment
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, false, nil, astroV1Client)
	if err != nil {
		return err
	}

	// build query input
	oldEnvironmentVariables := []astrov1.DeploymentEnvironmentVariable{}
	if currentDeployment.EnvironmentVariables != nil {
		oldEnvironmentVariables = *currentDeployment.EnvironmentVariables
	}

	newEnvironmentVariables := make([]astrov1.DeploymentEnvironmentVariableRequest, 0)
	oldKeyList := make([]string, 0)
	varErr := false

	// add old variables to update
	for i := range oldEnvironmentVariables {
		oldEnvironmentVariable := astrov1.DeploymentEnvironmentVariableRequest{
			IsSecret: oldEnvironmentVariables[i].IsSecret,
			Key:      oldEnvironmentVariables[i].Key,
			Value:    oldEnvironmentVariables[i].Value,
		}
		newEnvironmentVariables = append(newEnvironmentVariables, oldEnvironmentVariable)
		oldKeyList = append(oldKeyList, oldEnvironmentVariables[i].Key)
	}

	// add new variable from flag
	if variableKey != "" && variableValue != "" {
		newEnvironmentVariables = addVariable(oldKeyList, oldEnvironmentVariables, newEnvironmentVariables, variableKey, variableValue, updateVars, makeSecret, out)
	}
	if variableValue == "" && variableKey != "" {
		fmt.Fprintf(out, "Variable with key %s not created or updated\nYou must provide a variable value", variableKey)
		varErr = true
	}
	if variableValue != "" && variableKey == "" {
		fmt.Fprintf(out, "Variable with value %s not created or updated with flags\nYou must provide a variable key", variableValue)
		varErr = true
	}
	// add new variables from list of variables provided through args
	if len(variableList) > 0 {
		var listErr bool
		newEnvironmentVariables, listErr = addVariablesFromArgs(oldKeyList, oldEnvironmentVariables, newEnvironmentVariables, variableList, updateVars, makeSecret, out)
		varErr = varErr || listErr
	}
	// add new variables from file
	if useEnvFile {
		var fileErr bool
		newEnvironmentVariables, fileErr = addVariablesFromFile(envFile, oldKeyList, oldEnvironmentVariables, newEnvironmentVariables, updateVars, makeSecret)
		varErr = varErr || fileErr
	}

	// update deployment
	err = Update(currentDeployment.Id, "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", 0, 0, []astrov1.WorkerQueueRequest{}, []astrov1.HybridWorkerQueueRequest{}, newEnvironmentVariables, nil, nil, nil, false, astroV1Client)
	if err != nil {
		return err
	}
	deployment, err := GetDeploymentByID("", currentDeployment.Id, astroV1Client)
	if err != nil {
		return err
	}
	if deployment.EnvironmentVariables != nil {
		environmentVariablesObjects = *deployment.EnvironmentVariables
	}

	if len(environmentVariablesObjects) == 0 {
		fmt.Fprintln(out, "\nNo variables for this Deployment")
	} else {
		fmt.Fprintln(out, "\nUpdated list of your Deployment's variables:")
		makeVarTable(environmentVariablesObjects).Print(out) //nolint:errcheck // best-effort render to the terminal
	}
	if varErr {
		return errVarCreateUpdate
	}

	return nil
}

func contains(elems []string, v string) (exist bool, num int) {
	for i, s := range elems {
		if v == s {
			return true, i
		}
	}
	return false, 0
}

// readLines reads a whole file into memory and returns a slice of its lines.
func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// writes vars from cloud into a file
func writeVarToFile(environmentVariablesObjects []astrov1.DeploymentEnvironmentVariable, envFile string) error {
	f, err := os.OpenFile(envFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:mnd // the value is clear from context
	if err != nil {
		return err
	}

	defer f.Close()

	for _, variable := range environmentVariablesObjects {
		var value string
		if variable.IsSecret {
			value = " # secret"
		} else if variable.Value != nil {
			value = *variable.Value
		}
		_, err := f.WriteString("\n" + variable.Key + "=" + value)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to write variable %s to file:\n%s\n", variable.Key, err)
		}
	}
	fmt.Printf("\nThe following environment variables were saved to the file %s,\nsecret environment variables were saved only with a key:\n\n", envFile)
	return nil
}

// Add variables
func addVariable(oldKeyList []string, oldEnvironmentVariables []astrov1.DeploymentEnvironmentVariable, newEnvironmentVariables []astrov1.DeploymentEnvironmentVariableRequest, variableKey, variableValue string, updateVars, makeSecret bool, out io.Writer) []astrov1.DeploymentEnvironmentVariableRequest {
	var newEnvironmentVariable astrov1.DeploymentEnvironmentVariableRequest
	exist, num := contains(oldKeyList, variableKey)
	switch {
	case exist && !updateVars: // don't update variable
		fmt.Fprintf(out, "key %s already exists, skipping creation. Use the update command to update existing variables\n", variableKey)
	case exist && updateVars: // update variable
		fmt.Fprintf(out, "updating variable %s \n", variableKey)
		secret := makeSecret
		if !makeSecret { // you can only make variables secret a user can't make them not secret
			secret = oldEnvironmentVariables[num].IsSecret
		}
		newEnvironmentVariable = astrov1.DeploymentEnvironmentVariableRequest{
			IsSecret: secret,
			Key:      oldEnvironmentVariables[num].Key,
			Value:    &variableValue,
		}
		newEnvironmentVariables[num] = newEnvironmentVariable
	default:
		newFileEnvironmentVariable := astrov1.DeploymentEnvironmentVariableRequest{
			IsSecret: makeSecret,
			Key:      variableKey,
			Value:    &variableValue,
		}
		newEnvironmentVariables = append(newEnvironmentVariables, newFileEnvironmentVariable)
		fmt.Printf("adding variable %s\n", variableKey)
	}
	return newEnvironmentVariables
}

func addVariablesFromArgs(oldKeyList []string, oldEnvironmentVariables []astrov1.DeploymentEnvironmentVariable, newEnvironmentVariables []astrov1.DeploymentEnvironmentVariableRequest, variableList []string, updateVars, makeSecret bool, out io.Writer) ([]astrov1.DeploymentEnvironmentVariableRequest, bool) {
	var key string
	var val string
	varErr := false
	// validate each key-value pair and add it to the new variables list
	for i := range variableList {
		// split pair
		pair := strings.SplitN(variableList[i], "=", 2)
		if len(pair) == 2 {
			key = pair[0]
			val = pair[1]
			if key == "" || val == "" {
				fmt.Printf("Input %s has blank key or value\n", variableList[i])
				varErr = true
				continue
			}
		} else {
			fmt.Printf("Input %s is not a valid key value pair, should be of the form key=value\n", variableList[i])
			varErr = true
			continue
		}
		newEnvironmentVariables = addVariable(oldKeyList, oldEnvironmentVariables, newEnvironmentVariables, key, val, updateVars, makeSecret, out)
	}
	return newEnvironmentVariables, varErr
}

// Add variables from file
func addVariablesFromFile(envFile string, oldKeyList []string, oldEnvironmentVariables []astrov1.DeploymentEnvironmentVariable, newEnvironmentVariables []astrov1.DeploymentEnvironmentVariableRequest, updateVars, makeSecret bool) ([]astrov1.DeploymentEnvironmentVariableRequest, bool) {
	newKeyList := make([]string, 0)
	varErr := false
	vars, err := readLines(envFile)
	if err != nil {
		fmt.Printf("unable to read file %s :\n", envFile)
		fmt.Println(err)
	}
	for i := range vars {
		if strings.HasPrefix(vars[i], "#") {
			continue
		}
		if vars[i] == "" {
			continue
		}
		if len(strings.SplitN(vars[i], "=", 2)) == 1 {
			fmt.Printf("%s is an improperly formatted variable, no variable created\n", vars[i])
			varErr = true
			continue
		}
		key := strings.SplitN(vars[i], "=", 2)[0]
		value := strings.SplitN(vars[i], "=", 2)[1]
		if key == "" {
			fmt.Printf("empty key! skipping creating variable with key: %s\n", key)
			varErr = true
			continue
		}
		if value == "" {
			fmt.Printf("empty value! skipping creating variable with key: %s\n", key)
			varErr = true
			continue
		}
		// check if key is listed twice in file
		existFile, _ := contains(newKeyList, key)
		if existFile {
			fmt.Printf("key %s already exists within the file specified, skipping creation\n", key)
			varErr = true
			continue
		}

		fmt.Printf("Cleaning quotes and whitespaces from variable %s", key)
		value = strings.Trim(value, `"`)
		value = strings.Trim(value, `'`)
		value = strings.TrimSpace(value)

		// check if key already exists
		exist, num := contains(oldKeyList, key)
		if exist {
			if !updateVars { // only update a variable if a user specifys
				fmt.Printf("key %s already exists skipping creation use the --update flag to update old variables\n", key)
				varErr = true
				continue
			}
			// update variable
			fmt.Printf("updating variable %s \n", key)
			secret := makeSecret
			if !makeSecret { // you can only make variables secret a user can't make them not secret
				secret = oldEnvironmentVariables[num].IsSecret
			}

			newEnvironmentVariables[num] = astrov1.DeploymentEnvironmentVariableRequest{
				IsSecret: secret,
				Key:      oldEnvironmentVariables[num].Key,
				Value:    &value,
			}
			newKeyList = append(newKeyList, key)
			continue
		}
		newFileEnvironmentVariable := astrov1.DeploymentEnvironmentVariableRequest{
			IsSecret: makeSecret,
			Key:      key,
			Value:    &value,
		}
		newEnvironmentVariables = append(newEnvironmentVariables, newFileEnvironmentVariable)
		newKeyList = append(newKeyList, key)
		fmt.Printf("adding variable %s\n", key)
	}
	return newEnvironmentVariables, varErr
}
