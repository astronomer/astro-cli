package nexus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	sharedAPI "github.com/astronomer/nexus/api"
	"github.com/astronomer/nexus/app"
	"github.com/astronomer/nexus/restish"
	"github.com/astronomer/nexus/shared"

	"github.com/iancoleman/strcase"
	"golang.org/x/term"
)

func isTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

type inlineStatus struct {
	hasLine bool
	silent  bool
}

func newInlineStatus() *inlineStatus {
	return &inlineStatus{silent: !isTerminal()}
}

func (s *inlineStatus) Update(message string) {
	if s.silent {
		return
	}
	if s.hasLine {
		fmt.Fprintf(os.Stderr, "\r\033[K")
	}
	fmt.Fprintf(os.Stderr, "  %s", message)
	s.hasLine = true
}

func (s *inlineStatus) Done(message string) {
	if s.silent {
		return
	}
	if s.hasLine {
		fmt.Fprintf(os.Stderr, "\r\033[K")
	}
	fmt.Fprintln(os.Stderr, message)
	s.hasLine = false
}

var errSilent = fmt.Errorf("")

func (n *nexusCLI) runRestishCommand(command string, args []string) error {
	api := n.ctx.Config.GetDefaultAPI()
	if api == "" {
		fmt.Fprintln(os.Stderr, "Error: no default API. Set with 'nexus config set api <api>'")
		return errSilent
	}

	var positionalArgs []string
	bodyFields := make(map[string]string)
	var rawBody string
	var prNumber string
	var verbose bool

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") || strings.HasPrefix(arg, "-") {
			if arg == "--help" || arg == "-h" {
				n.showCommandHelp(api, command)
				return nil
			}
			if arg == "--verbose" || arg == "-v" {
				verbose = true
				continue
			}

			key := strings.TrimPrefix(strings.TrimPrefix(arg, "--"), "-")
			var val string
			if idx := strings.Index(key, "="); idx != -1 {
				val = key[idx+1:]
				key = key[:idx]
			} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				val = args[i]
			}

			switch key {
			case "body":
				rawBody = val
			case "pr":
				prNumber = val
			case "o", "output":
			default:
				bodyFields[key] = val
			}
		} else {
			positionalArgs = append(positionalArgs, arg)
		}
	}

	if positionalArgs == nil {
		positionalArgs = []string{}
	}
	filledArgs, filledFromConfig := n.fillPositionalArgs(api, command, positionalArgs)
	if filledArgs == nil {
		return errSilent
	}

	if !n.confirmDestructiveAction(command, filledFromConfig) {
		fmt.Fprintln(os.Stderr, "Canceled.")
		return errSilent
	}

	status := newInlineStatus()
	body := n.buildRequestBody(api, command, rawBody, bodyFields, filledArgs, status)

	status.Update("Executing " + command + "...")
	result, err := n.executeRestishCommand(api, command, filledArgs, body, prNumber, verbose)
	if err != nil {
		status.Done("Error:")
		fmt.Fprintln(os.Stderr, err.Error())
		return errSilent
	}

	status.Done(shared.StatusSuccess)

	if result != "" && result != "null" {
		fmt.Println(result)
	}

	n.updateConfigAfterCommand(api, command, result, sharedAPI.GetDeletedID(filledArgs, command))
	return nil
}

func (n *nexusCLI) confirmDestructiveAction(command string, filledFromConfig map[string]string) bool {
	if len(filledFromConfig) == 0 {
		return true
	}

	isDestructive := strings.HasPrefix(command, "update-") || strings.HasPrefix(command, "delete-")
	if !isDestructive {
		return true
	}

	if !isTerminal() {
		return true
	}

	action := "update"
	if strings.HasPrefix(command, "delete-") {
		action = "delete"
	}

	fmt.Fprintln(os.Stderr, "Using defaults from config:")
	for resourceType, id := range filledFromConfig {
		fmt.Fprintf(os.Stderr, "  %s: %s\n", resourceType, id)
	}
	fmt.Fprintf(os.Stderr, "\nProceed with %s? [y/N]: ", action)

	var response string
	_, _ = fmt.Scanln(&response)
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

func (n *nexusCLI) fillPositionalArgs(api, command string, provided []string) (filled []string, fromConfig map[string]string) {
	parser, err := restish.NewHelpParser(api, command)
	if err != nil {
		return provided, nil
	}

	expectedArgs := parser.ParseExpectedArgs()
	if len(expectedArgs) == 0 || len(provided) >= len(expectedArgs) {
		return provided, nil
	}

	filled = make([]string, len(provided))
	copy(filled, provided)
	fromConfig = make(map[string]string)

	for i := len(provided); i < len(expectedArgs); i++ {
		argName := strings.ToLower(expectedArgs[i])
		var defaultVal string
		var resourceType string

		switch {
		case strings.Contains(argName, shared.ResourceOrganization):
			defaultVal = n.ctx.Config.GetDefaultOrganizationForAPI(api)
			resourceType = shared.ResourceOrganization
		case strings.Contains(argName, shared.ResourceWorkspace):
			defaultVal = n.ctx.Config.GetDefaultWorkspaceForAPI(api)
			resourceType = shared.ResourceWorkspace
		case strings.Contains(argName, shared.ResourceDeployment):
			defaultVal = n.ctx.Config.GetDefaultDeploymentForAPI(api)
			resourceType = shared.ResourceDeployment
		}

		if defaultVal == "" {
			fmt.Fprintf(os.Stderr, "Error: missing required argument '%s'\n", expectedArgs[i])
			fmt.Fprintf(os.Stderr, "  Set a default with: nexus config set %s <id>\n", resourceType)
			return nil, nil
		}

		filled = append(filled, defaultVal)
		if resourceType != "" {
			fromConfig[resourceType] = defaultVal
		}
	}

	return filled, fromConfig
}

func (n *nexusCLI) createSourceFetcher(api, command string, positionalArgs []string) *app.SourceFetcher {
	availableArgs := make(map[string]string)

	// Map positional arg values to their expected names
	if len(positionalArgs) > 0 {
		parser, err := restish.NewHelpParser(api, command)
		if err == nil {
			expectedArgs := parser.ParseExpectedArgs()
			for i, name := range expectedArgs {
				if i < len(positionalArgs) {
					availableArgs[name] = positionalArgs[i]
					availableArgs[strcase.ToLowerCamel(name)] = positionalArgs[i]
				}
			}
		}
	}

	// Fill from config defaults if not already provided
	orgID := n.ctx.Config.GetDefaultOrganizationForAPI(api)
	if orgID != "" {
		if _, ok := availableArgs["organization-id"]; !ok {
			availableArgs["organization-id"] = orgID
			availableArgs["organizationId"] = orgID
		}
	}

	fetcher := app.NewSourceFetcher(api, availableArgs)

	var authDomain string
	if meta, ok := n.ctx.APIs.Get(api); ok {
		authDomain = meta.AuthDomain
	}

	fetcher.RegisterFetcher("userinfo", func() map[string]any {
		email := sharedAPI.FetchCurrentUserEmail(api, authDomain)
		if email == "" {
			return nil
		}
		return map[string]any{"email": email}
	})
	return fetcher
}

func (n *nexusCLI) buildRequestBody(api, command, rawBody string, bodyFields map[string]string, positionalArgs []string, status *inlineStatus) string {
	if rawBody != "" {
		return rawBody
	}

	schemaParser, err := restish.NewSchemaParser(api, command)
	if err != nil {
		if len(bodyFields) == 0 {
			return ""
		}
		camelFields := make(map[string]string)
		for k, v := range bodyFields {
			camelFields[strcase.ToLowerCamel(k)] = v
		}
		bodyJSON, _ := json.Marshal(camelFields)
		return string(bodyJSON)
	}

	fields := schemaParser.GetFields(0)
	if len(fields) == 0 && len(bodyFields) == 0 {
		return ""
	}

	fetcher := n.createSourceFetcher(api, command, positionalArgs)
	fetcher.OnStatus = status.Update
	registry := n.ctx.APIs

	fieldTypes := make(map[string]string)
	for _, f := range fields {
		fieldTypes[strings.ToLower(f.Name)] = f.Type
	}

	bodyData := make(map[string]any)

	for k, v := range bodyFields {
		camelKey := strcase.ToLowerCamel(k)
		fieldType := fieldTypes[strings.ToLower(strcase.ToLowerCamel(k))]
		bodyData[camelKey] = convertToType(v, fieldType)
	}

	configFields := make(map[string]bool)
	for _, f := range fields {
		camelKey := strcase.ToLowerCamel(f.Name)
		if _, exists := bodyData[camelKey]; exists {
			continue
		}

		if val, ok := registry.GetFieldValue(api, command, f.Name); ok {
			configFields[camelKey] = true
			defaultVal := fetcher.ResolveValue(val)
			if defaultVal != "" {
				bodyData[camelKey] = convertToType(defaultVal, f.Type)
				continue
			}
		}

		if defaultVal := n.getConfigDefault(f.Name, api); defaultVal != "" && f.Required {
			bodyData[camelKey] = convertToType(defaultVal, f.Type)
			continue
		}

		if f.Required {
			if defaultVal := shared.GetFieldDefaultByType(f.Type, f.Enum); defaultVal != "" {
				bodyData[camelKey] = convertToType(defaultVal, f.Type)
			}
		}
	}

	if len(bodyData) == 0 {
		return ""
	}

	bodyJSON, err := json.Marshal(bodyData)
	if err != nil {
		return ""
	}
	return string(bodyJSON)
}

func (n *nexusCLI) getConfigDefault(fieldName, api string) string {
	fieldLower := strings.ToLower(fieldName)
	if strings.Contains(fieldLower, shared.ResourceWorkspace) && strings.Contains(fieldLower, "id") {
		return n.ctx.Config.GetDefaultWorkspaceForAPI(api)
	}
	return ""
}

func convertToType(value, fieldType string) any {
	switch strings.ToLower(fieldType) {
	case "boolean", "bool":
		return value == "true"
	case "integer", "int", "int64":
		if n, err := json.Number(value).Int64(); err == nil {
			return n
		}
		return value
	case "number", "float", "float64":
		if f, err := json.Number(value).Float64(); err == nil {
			return f
		}
		return value
	case "array":
		var arr []any
		if err := json.Unmarshal([]byte(value), &arr); err == nil {
			return arr
		}
		return value
	case "object":
		var obj map[string]any
		if err := json.Unmarshal([]byte(value), &obj); err == nil {
			return obj
		}
		return value
	default:
		return value
	}
}

func (n *nexusCLI) executeRestishCommand(api, command string, args []string, body, prNumber string, verbose bool) (string, error) {
	restishArgs := []string{api, command}
	restishArgs = append(restishArgs, args...)

	if body != "" {
		restishArgs = append(restishArgs, body)
	}

	if prNumber != "" {
		serverURL := n.ctx.APIs.GetPRPreviewServerURL(api, prNumber)
		if serverURL != "" {
			restishArgs = append(restishArgs, "--rsh-server", serverURL)
		}
	}

	if verbose {
		cmdArgs := []string{api, command}
		cmdArgs = append(cmdArgs, args...)
		fmt.Fprintf(os.Stderr, "Command: restish %s\n", strings.Join(cmdArgs, " "))

		if body != "" {
			fmt.Fprintln(os.Stderr, "Body:")
			var prettyBody bytes.Buffer
			if err := json.Indent(&prettyBody, []byte(body), "  ", "  "); err == nil {
				fmt.Fprintln(os.Stderr, "  "+prettyBody.String())
			} else {
				fmt.Fprintln(os.Stderr, "  "+body)
			}
		}
		fmt.Fprintln(os.Stderr)
	}

	output, err := sharedAPI.ExecuteSimple(restishArgs)
	if err != nil {
		return "", fmt.Errorf("%s", output)
	}

	trimmed := strings.TrimSpace(output)

	return trimmed, nil
}

func (n *nexusCLI) updateConfigAfterCommand(apiName, command, result, deletedID string) {
	var response map[string]any
	if result != "" && result != "null" {
		if err := json.Unmarshal([]byte(result), &response); err == nil {
			if _, hasMsg := response["message"]; hasMsg {
				if _, hasCode := response["statusCode"]; hasCode {
					response = nil
				}
			}
		}
	}

	updater := sharedAPI.NewConfigUpdater(n.ctx.Config)
	updated, resourceType, resourceID := updater.UpdateAfterCommand(apiName, command, response, deletedID)

	if updated {
		if strings.HasPrefix(command, "delete-") {
			fmt.Fprintf(os.Stderr, "Cleared default %s from config\n", resourceType)
		} else {
			action := "Set"
			if strings.HasPrefix(command, "update-") {
				action = "Updated"
			}
			fmt.Fprintf(os.Stderr, "%s default %s %s in config\n", action, resourceType, resourceID)
		}
	}
}

func (n *nexusCLI) showCommandHelp(api, command string) {
	fmt.Printf("Help for %s\n\n", command)

	fmt.Println("Usage:")
	fmt.Printf("  nexus %s [args...] [flags]\n", command)

	n.printBodyFields(api, command)
	printQueryFlags(api, command)

	fmt.Println()
	fmt.Println("Use --body='{}' for raw JSON input.")
}

func (n *nexusCLI) printBodyFields(api, command string) {
	schemaParser, err := restish.NewSchemaParser(api, command)
	if err != nil {
		return
	}

	fields := schemaParser.GetFields(-1)
	if len(fields) == 0 {
		return
	}

	fetcher := n.createSourceFetcher(api, command, nil)
	registry := n.ctx.APIs

	fmt.Println()
	fmt.Println("Body Fields:")

	for _, f := range fields {
		desc := formatFieldDescription(f)
		defaultVal := n.resolveFieldDefault(api, command, f.Name, registry, fetcher)
		if defaultVal != "" {
			desc += " [default: " + defaultVal + "]"
		}

		req := ""
		if f.Required {
			req = " (required)"
		}

		fmt.Printf("  --%-25s  %s%s\n", strcase.ToKebab(f.Name), desc, req)
	}
}

func (n *nexusCLI) resolveFieldDefault(api, command, fieldName string, registry *app.APIRegistry, fetcher *app.SourceFetcher) string {
	if val, ok := registry.GetFieldValue(api, command, fieldName); ok {
		if resolved := fetcher.ResolveValue(val); resolved != "" {
			return resolved
		}
	}

	return n.getConfigDefault(fieldName, api)
}

func formatFieldDescription(f restish.SchemaField) string {
	if f.Description != "" {
		if f.Enum != "" {
			return f.Description + " (one of: " + f.Enum + ")"
		}

		return f.Description
	}

	if f.Enum != "" {
		return "One of: " + f.Enum
	}

	return f.Type
}

func printQueryFlags(api, command string) {
	helpParser, err := restish.NewHelpParser(api, command)
	if err != nil {
		return
	}

	flags := helpParser.ParseFlagsSection()
	if len(flags) == 0 {
		return
	}

	fmt.Println()
	fmt.Println("Query Flags:")

	for _, f := range flags {
		fmt.Printf("  --%-25s  %s\n", f.Name, f.Description)
	}
}
