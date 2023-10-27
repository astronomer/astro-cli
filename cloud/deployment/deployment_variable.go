package deployment

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	astro "github.com/astronomer/astro-cli/astro-client"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/pkg/errors"
)

var (
	errVarBool                  = false
	errVarCreateUpdate          = errors.New("there was an error while creating or updating one or more of the environment variables. Check the logs above for more information")
	environmentVariablesObjects = []astroplatformcore.DeploymentEnvironmentVariable{}
)

func VariableList(deploymentID, variableKey, ws, envFile, deploymentName string, useEnvFile bool, platformCoreClient astroplatformcore.CoreClient, client astro.Client, out io.Writer) error {
	varTab := printutil.Table{
		Padding:        []int{5, 30, 30, 50},
		DynamicPadding: true,
		Header:         []string{"#", "KEY", "VALUE", "SECRET"},
	}

	// get deployment
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, false, client, platformCoreClient, nil)
	if err != nil {
		return err
	}

	if currentDeployment.EnvironmentVariables != nil {
		environmentVariablesObjects = *currentDeployment.EnvironmentVariables
	}

	// open env file
	if useEnvFile {
		err = writeVarToFile(environmentVariablesObjects, variableKey, envFile)
		if err != nil {
			fmt.Fprintln(out, errors.Wrap(err, "unable to write environment variables to file"))
		}
	}

	var nbEnvVarFound int
	for i := range environmentVariablesObjects {
		if environmentVariablesObjects[i].Key == variableKey {
			nbEnvVarFound++
			varTab.AddRow([]string{strconv.Itoa(nbEnvVarFound), environmentVariablesObjects[i].Key, *environmentVariablesObjects[i].Value, strconv.FormatBool(environmentVariablesObjects[i].IsSecret)}, false)
			break
		} else if variableKey == "" {
			nbEnvVarFound++
			varTab.AddRow([]string{strconv.Itoa(nbEnvVarFound), environmentVariablesObjects[i].Key, *environmentVariablesObjects[i].Value, strconv.FormatBool(environmentVariablesObjects[i].IsSecret)}, false)
		}
	}

	if nbEnvVarFound == 0 {
		fmt.Fprintln(out, "\nNo variables found")
		return nil
	}
	varTab.Print(out)

	return nil
}

// this function modifies a deployment's environment variable object
// it is used to create and update deployment's environment variables
func VariableModify(deploymentID, variableKey, variableValue, ws, envFile, deploymentName string, variableList []string, useEnvFile, makeSecret, updateVars bool, platformCoreClient astroplatformcore.CoreClient, client astro.Client, out io.Writer) error {
	varTab := printutil.Table{
		Padding:        []int{5, 30, 30, 50},
		DynamicPadding: true,
		Header:         []string{"#", "KEY", "VALUE", "SECRET"},
	}

	// get deployment
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, false, client, platformCoreClient, nil)
	if err != nil {
		return err
	}

	// build query input
	var oldEnvironmentVariables = []astroplatformcore.DeploymentEnvironmentVariable{}
	if currentDeployment.EnvironmentVariables != nil {
		oldEnvironmentVariables = *currentDeployment.EnvironmentVariables
	}

	newEnvironmentVariables := make([]astro.EnvironmentVariable, 0)
	oldKeyList := make([]string, 0)

	// add old variables to update
	for i := range oldEnvironmentVariables {
		oldEnvironmentVariable := astro.EnvironmentVariable{
			IsSecret: oldEnvironmentVariables[i].IsSecret,
			Key:      oldEnvironmentVariables[i].Key,
			Value:    *oldEnvironmentVariables[i].Value,
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
		errVarBool = true
	}
	if variableValue != "" && variableKey == "" {
		fmt.Fprintf(out, "Variable with value %s not created or updated with flags\nYou must provide a variable key", variableValue)
		errVarBool = true
	}
	// add new variables from list of variables provided through args
	if len(variableList) > 0 {
		newEnvironmentVariables = addVariablesFromArgs(oldKeyList, oldEnvironmentVariables, newEnvironmentVariables, variableList, updateVars, makeSecret, out)
	}
	// add new variables from file
	if useEnvFile {
		newEnvironmentVariables = addVariablesFromFile(envFile, oldKeyList, oldEnvironmentVariables, newEnvironmentVariables, updateVars, makeSecret)
	}

	// create variable input
	variablesCreateInput := astro.EnvironmentVariablesInput{
		DeploymentID:         currentDeployment.Id,
		EnvironmentVariables: newEnvironmentVariables,
	}

	// update deployment
	environmentVariablesObjects, err := client.ModifyDeploymentVariable(variablesCreateInput)
	if err != nil {
		return errors.Wrap(err, astro.AstronomerConnectionErrMsg)
	}

	// make variables table
	var index int
	for i := range environmentVariablesObjects {
		index = i + 1
		varTab.AddRow([]string{strconv.Itoa(index), environmentVariablesObjects[i].Key, environmentVariablesObjects[i].Value, strconv.FormatBool(environmentVariablesObjects[i].IsSecret)}, false)
	}

	if index == 0 {
		fmt.Fprintln(out, "\nNo variables for this Deployment")
	} else {
		fmt.Fprintln(out, "\nUpdated list of your Deployment's variables:")
		varTab.Print(out)
	}
	if errVarBool {
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
func writeVarToFile(environmentVariablesObjects []astroplatformcore.DeploymentEnvironmentVariable, variableKey, envFile string) error {
	f, err := os.OpenFile(envFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gomnd
	if err != nil {
		return err
	}

	defer f.Close()

	for i := range environmentVariablesObjects {
		if environmentVariablesObjects[i].Key == variableKey {
			_, err := f.WriteString("\n" + environmentVariablesObjects[i].Key + "=" + *environmentVariablesObjects[i].Value)
			if err != nil {
				fmt.Println("unable to write variable " + environmentVariablesObjects[i].Key + " to file:")
				fmt.Println(err)
			}
		} else if variableKey == "" {
			_, err := f.WriteString("\n" + environmentVariablesObjects[i].Key + "=" + *environmentVariablesObjects[i].Value)
			if err != nil {
				fmt.Println("unable to write variable " + environmentVariablesObjects[i].Key + " to file:")
				fmt.Println(err)
			}
		}
	}
	fmt.Println("\nThe following environment variables were saved to the file " + envFile + ",\nsecret environment variables were saved only with a key:\n")
	return nil
}

// Add variables
func addVariable(oldKeyList []string, oldEnvironmentVariables []astroplatformcore.DeploymentEnvironmentVariable, newEnvironmentVariables []astro.EnvironmentVariable, variableKey, variableValue string, updateVars, makeSecret bool, out io.Writer) []astro.EnvironmentVariable {
	var newEnvironmentVariable astro.EnvironmentVariable
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
		newEnvironmentVariable = astro.EnvironmentVariable{
			IsSecret: secret,
			Key:      oldEnvironmentVariables[num].Key,
			Value:    variableValue,
		}
		newEnvironmentVariables[num] = newEnvironmentVariable
	default:
		newFileEnvironmentVariable := astro.EnvironmentVariable{
			IsSecret: makeSecret,
			Key:      variableKey,
			Value:    variableValue,
		}
		newEnvironmentVariables = append(newEnvironmentVariables, newFileEnvironmentVariable)
		fmt.Printf("adding variable %s\n", variableKey)
	}
	return newEnvironmentVariables
}

func addVariablesFromArgs(oldKeyList []string, oldEnvironmentVariables []astroplatformcore.DeploymentEnvironmentVariable, newEnvironmentVariables []astro.EnvironmentVariable, variableList []string, updateVars, makeSecret bool, out io.Writer) []astro.EnvironmentVariable {
	var key string
	var val string
	// validate each key-value pair and add it to the new variables list
	for i := range variableList {
		// split pair
		pair := strings.Split(variableList[i], "=")
		if len(pair) > 1 {
			key = pair[0]
			val = pair[1]
			if key == "" || val == "" {
				fmt.Printf("Input %s has blank key or value\n", variableList[i])
				errVarBool = true
				continue
			}
		} else {
			fmt.Printf("Input %s is not a valid key value pair, should be of the form key=value\n", variableList[i])
			errVarBool = true
			continue
		}
		newEnvironmentVariables = addVariable(oldKeyList, oldEnvironmentVariables, newEnvironmentVariables, key, val, updateVars, makeSecret, out)
	}
	return newEnvironmentVariables
}

// Add variables from file
func addVariablesFromFile(envFile string, oldKeyList []string, oldEnvironmentVariables []astroplatformcore.DeploymentEnvironmentVariable, newEnvironmentVariables []astro.EnvironmentVariable, updateVars, makeSecret bool) []astro.EnvironmentVariable {
	newKeyList := make([]string, 0)
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
		if len(strings.SplitN(vars[i], "=", 2)) == 1 { //nolint:gomnd
			fmt.Printf("%s is an improperly formatted variable, no variable created\n", vars[i])
			errVarBool = true
			continue
		}
		key := strings.SplitN(vars[i], "=", 2)[0]   //nolint:gomnd
		value := strings.SplitN(vars[i], "=", 2)[1] //nolint:gomnd
		if key == "" {
			fmt.Printf("empty key! skipping creating variable with key: %s\n", key)
			errVarBool = true
			continue
		}
		if value == "" {
			fmt.Printf("empty value! skipping creating variable with key: %s\n", key)
			errVarBool = true
			continue
		}
		// check if key is listed twice in file
		existFile, _ := contains(newKeyList, key)
		if existFile {
			fmt.Printf("key %s already exists within the file specified, skipping creation\n", key)
			errVarBool = true
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
				errVarBool = true
				continue
			}
			// update variable
			fmt.Printf("updating variable %s \n", key)
			secret := makeSecret
			if !makeSecret { // you can only make variables secret a user can't make them not secret
				secret = oldEnvironmentVariables[num].IsSecret
			}

			newEnvironmentVariables[num] = astro.EnvironmentVariable{
				IsSecret: secret,
				Key:      oldEnvironmentVariables[num].Key,
				Value:    value,
			}
			newKeyList = append(newKeyList, key)
			continue
		}
		newFileEnvironmentVariable := astro.EnvironmentVariable{
			IsSecret: makeSecret,
			Key:      key,
			Value:    value,
		}
		newEnvironmentVariables = append(newEnvironmentVariables, newFileEnvironmentVariable)
		newKeyList = append(newKeyList, key)
		fmt.Printf("adding variable %s\n", key)
	}
	return newEnvironmentVariables
}
