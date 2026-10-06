package deployment

import (
	"bufio"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/pkg/errors"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

// VariableList returns a Deployment's environment variables, optionally
// narrowed to one key.
//
// When useEnvFile is set it also appends them to envFile, and reports the
// count written so cmd can say so. A write that fails is an error naming the
// file, not a line on stderr: the caller decides whether a partial file is
// fatal.
func VariableList(deploymentID, variableKey, ws, envFile, deploymentName string, useEnvFile bool, astroV1Client astrov1.APIClient) (*DeploymentVariables, error) {
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, false, nil, astroV1Client)
	if err != nil {
		return nil, err
	}

	objs := []astrov1.DeploymentEnvironmentVariable{}
	if currentDeployment.EnvironmentVariables != nil {
		objs = *currentDeployment.EnvironmentVariables
	}
	if variableKey != "" {
		objs = slices.DeleteFunc(objs, func(v astrov1.DeploymentEnvironmentVariable) bool {
			return v.Key != variableKey
		})
	}

	if useEnvFile {
		if err := writeVarToFile(objs, envFile); err != nil {
			return nil, errors.Wrapf(err, "unable to write environment variables to %s", envFile)
		}
	}

	return &DeploymentVariables{Variables: toVariableInfo(objs)}, nil
}

// toVariableInfo converts the API's variables into the value cmd renders. A
// secret's value is dropped rather than masked: masking is a text-rendering
// choice, and a JSON caller should get no value at all rather than a string of
// asterisks it might mistake for one.
func toVariableInfo(vars []astrov1.DeploymentEnvironmentVariable) []VariableInfo {
	out := make([]VariableInfo, 0, len(vars))
	for i := range vars {
		info := VariableInfo{Key: vars[i].Key, IsSecret: vars[i].IsSecret}
		if !vars[i].IsSecret && vars[i].Value != nil {
			info.Value = *vars[i].Value
		}
		out = append(out, info)
	}
	return out
}

// VariableModify creates or updates a Deployment's environment variables from
// a key/value pair, a list of `key=value` arguments, an env file, or any
// combination, and reports what happened to each input.
//
// It returns a result even when some inputs were invalid: the Deployment is
// still updated with the ones that were usable, and the caller decides what to
// do about the rest. Only a failure to reach the Deployment is an error.
func VariableModify(
	deploymentID, variableKey, variableValue, ws, envFile, deploymentName string,
	variableList []string,
	useEnvFile, makeSecret, updateVars bool,
	astroV1Client astrov1.APIClient,
) (*VariableModifyResult, error) {
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, false, nil, astroV1Client)
	if err != nil {
		return nil, err
	}

	oldEnvironmentVariables := []astrov1.DeploymentEnvironmentVariable{}
	if currentDeployment.EnvironmentVariables != nil {
		oldEnvironmentVariables = *currentDeployment.EnvironmentVariables
	}

	newEnvironmentVariables := make([]astrov1.DeploymentEnvironmentVariableRequest, 0, len(oldEnvironmentVariables))
	oldKeyList := make([]string, 0, len(oldEnvironmentVariables))
	for i := range oldEnvironmentVariables {
		newEnvironmentVariables = append(newEnvironmentVariables, astrov1.DeploymentEnvironmentVariableRequest{
			IsSecret: oldEnvironmentVariables[i].IsSecret,
			Key:      oldEnvironmentVariables[i].Key,
			Value:    oldEnvironmentVariables[i].Value,
		})
		oldKeyList = append(oldKeyList, oldEnvironmentVariables[i].Key)
	}

	// Both lists start empty rather than nil: they are published as json, where
	// a list is always an array, never null.
	result := &VariableModifyResult{Outcomes: []VariableOutcome{}, Variables: []VariableInfo{}}

	switch {
	case variableKey != "" && variableValue != "":
		newEnvironmentVariables = addVariable(oldKeyList, oldEnvironmentVariables, newEnvironmentVariables,
			variableKey, variableValue, updateVars, makeSecret, result)
	case variableKey != "" && variableValue == "":
		result.Outcomes = append(result.Outcomes, VariableOutcome{
			Kind:   VariableInvalid,
			Key:    variableKey,
			Reason: "no value given; a variable needs both a key and a value",
		})
	case variableValue != "" && variableKey == "":
		result.Outcomes = append(result.Outcomes, VariableOutcome{
			Kind:   VariableInvalid,
			Input:  variableValue,
			Reason: "no key given; a variable needs both a key and a value",
		})
	}

	if len(variableList) > 0 {
		newEnvironmentVariables = addVariablesFromArgs(oldKeyList, oldEnvironmentVariables, newEnvironmentVariables,
			variableList, updateVars, makeSecret, result)
	}
	if useEnvFile {
		newEnvironmentVariables = addVariablesFromFile(envFile, oldKeyList, oldEnvironmentVariables,
			newEnvironmentVariables, updateVars, makeSecret, result)
	}

	err = Update(currentDeployment.Id, "", "", "", "", "", "", "", "", "", "", "", "", "", "", "",
		0, 0, []astrov1.WorkerQueueRequest{}, []astrov1.HybridWorkerQueueRequest{}, newEnvironmentVariables,
		nil, nil, nil, false, astroV1Client)
	if err != nil {
		return nil, err
	}
	updated, err := GetDeploymentByID("", currentDeployment.Id, astroV1Client)
	if err != nil {
		return nil, err
	}
	if updated.EnvironmentVariables != nil {
		result.Variables = toVariableInfo(*updated.EnvironmentVariables)
	}

	return result, nil
}

func contains(elems []string, v string) (exist bool, num int) {
	for i, s := range elems {
		if v == s {
			return true, i
		}
	}
	return false, 0
}

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

// writeVarToFile appends the variables to envFile, secrets by key only.
//
// A single failed write used to print to stderr and carry on, which left a
// half-written file and a zero exit. It now stops at the first failure and
// names the variable, so the caller can tell a complete file from a partial
// one.
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
		if _, err := f.WriteString("\n" + variable.Key + "=" + value); err != nil {
			return errors.Wrapf(err, "writing variable %s", variable.Key)
		}
	}
	return nil
}

// addVariable records one input against the Deployment's existing keys and
// appends its outcome to result.
func addVariable(
	oldKeyList []string,
	oldEnvironmentVariables []astrov1.DeploymentEnvironmentVariable,
	newEnvironmentVariables []astrov1.DeploymentEnvironmentVariableRequest,
	variableKey, variableValue string,
	updateVars, makeSecret bool,
	result *VariableModifyResult,
) []astrov1.DeploymentEnvironmentVariableRequest {
	exist, num := contains(oldKeyList, variableKey)
	switch {
	case exist && !updateVars:
		result.Outcomes = append(result.Outcomes, VariableOutcome{
			Kind:   VariableSkippedExists,
			Key:    variableKey,
			Reason: "already set; use the update command to change it",
		})
	case exist && updateVars:
		// A variable can be made secret but never made public again, so an
		// update keeps the old flag unless this run asks for secret.
		secret := makeSecret
		if !makeSecret {
			secret = oldEnvironmentVariables[num].IsSecret
		}
		newEnvironmentVariables[num] = astrov1.DeploymentEnvironmentVariableRequest{
			IsSecret: secret,
			Key:      oldEnvironmentVariables[num].Key,
			Value:    &variableValue,
		}
		result.Outcomes = append(result.Outcomes, VariableOutcome{Kind: VariableUpdated, Key: variableKey})
	default:
		newEnvironmentVariables = append(newEnvironmentVariables, astrov1.DeploymentEnvironmentVariableRequest{
			IsSecret: makeSecret,
			Key:      variableKey,
			Value:    &variableValue,
		})
		result.Outcomes = append(result.Outcomes, VariableOutcome{Kind: VariableCreated, Key: variableKey})
	}
	return newEnvironmentVariables
}

// addVariablesFromArgs validates each `key=value` argument and adds the usable
// ones, recording an outcome for every input either way.
func addVariablesFromArgs(
	oldKeyList []string,
	oldEnvironmentVariables []astrov1.DeploymentEnvironmentVariable,
	newEnvironmentVariables []astrov1.DeploymentEnvironmentVariableRequest,
	variableList []string,
	updateVars, makeSecret bool,
	result *VariableModifyResult,
) []astrov1.DeploymentEnvironmentVariableRequest {
	for i := range variableList {
		pair := strings.SplitN(variableList[i], "=", 2)
		if len(pair) != 2 {
			result.Outcomes = append(result.Outcomes, VariableOutcome{
				Kind:   VariableInvalid,
				Input:  variableList[i],
				Reason: "not a key=value pair",
			})
			continue
		}
		key, val := pair[0], pair[1]
		if key == "" || val == "" {
			result.Outcomes = append(result.Outcomes, VariableOutcome{
				Kind:   VariableInvalid,
				Input:  variableList[i],
				Key:    key,
				Reason: "blank key or value",
			})
			continue
		}
		newEnvironmentVariables = addVariable(oldKeyList, oldEnvironmentVariables, newEnvironmentVariables,
			key, val, updateVars, makeSecret, result)
	}
	return newEnvironmentVariables
}

// addVariablesFromFile reads envFile and adds the variables it declares.
//
// A file that cannot be read is one invalid outcome naming the file, not a
// silent skip: the run continues so the flag-supplied variables still land.
func addVariablesFromFile(
	envFile string,
	oldKeyList []string,
	oldEnvironmentVariables []astrov1.DeploymentEnvironmentVariable,
	newEnvironmentVariables []astrov1.DeploymentEnvironmentVariableRequest,
	updateVars, makeSecret bool,
	result *VariableModifyResult,
) []astrov1.DeploymentEnvironmentVariableRequest {
	vars, err := readLines(envFile)
	if err != nil {
		result.Outcomes = append(result.Outcomes, VariableOutcome{
			Kind:   VariableInvalid,
			Input:  envFile,
			Reason: "unable to read file: " + err.Error(),
		})
		return newEnvironmentVariables
	}

	fileKeys := make([]string, 0, len(vars))
	for i := range vars {
		if strings.HasPrefix(vars[i], "#") || vars[i] == "" {
			continue
		}
		pair := strings.SplitN(vars[i], "=", 2)
		if len(pair) != 2 {
			result.Outcomes = append(result.Outcomes, VariableOutcome{
				Kind:   VariableInvalid,
				Input:  vars[i],
				Reason: "not a key=value pair",
			})
			continue
		}
		key, value := pair[0], pair[1]
		switch {
		case key == "":
			// Not the line itself: its value may be a secret, and the error
			// carries Input to stderr and CI logs.
			result.Outcomes = append(result.Outcomes, VariableOutcome{
				Kind: VariableInvalid, Input: fmt.Sprintf("%s line %d", envFile, i+1), Reason: "blank key",
			})
			continue
		case value == "":
			result.Outcomes = append(result.Outcomes, VariableOutcome{
				Kind: VariableInvalid, Input: vars[i], Key: key, Reason: "blank value",
			})
			continue
		}
		if exist, _ := contains(fileKeys, key); exist {
			result.Outcomes = append(result.Outcomes, VariableOutcome{
				Kind: VariableInvalid, Key: key,
				Reason: "declared twice in the file",
			})
			continue
		}
		fileKeys = append(fileKeys, key)

		// A file value may be quoted for the shell's sake; the API wants the
		// value itself. Args do not get this, because a shell has already
		// removed their quotes by the time they reach us.
		value = strings.Trim(value, `"`)
		value = strings.Trim(value, `'`)
		value = strings.TrimSpace(value)

		// A file key the Deployment already has fails the run unless this is
		// an update, where an arg key is only skipped. That difference is
		// older than this shape and scripts may rely on the exit code.
		if exist, _ := contains(oldKeyList, key); exist && !updateVars {
			result.Outcomes = append(result.Outcomes, VariableOutcome{
				Kind: VariableInvalid, Key: key,
				Reason: "already set on the Deployment; use the update command to change it",
			})
			continue
		}

		newEnvironmentVariables = addVariable(oldKeyList, oldEnvironmentVariables, newEnvironmentVariables,
			key, value, updateVars, makeSecret, result)
	}
	return newEnvironmentVariables
}
