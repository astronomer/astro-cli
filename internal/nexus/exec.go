package nexus

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
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
		fmt.Fprintln(os.Stderr, "Error: no API configured. Use 'astro context switch' to select a domain.")
		return errSilent
	}

	var positionalArgs []string
	bodyFields := make(map[string]string)
	var rawBody string
	var prNumber string
	var verbose bool
	var forceFlag bool

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
			if arg == "--force" || arg == "-f" {
				forceFlag = true
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

	if !n.confirmDestructiveAction(command, filledArgs, filledFromConfig, forceFlag) {
		fmt.Fprintln(os.Stderr, "Canceled.")
		return errSilent
	}

	status := newInlineStatus()
	body := n.buildRequestBody(api, command, rawBody, bodyFields, filledArgs, status)

	status.Update("Executing " + command + "...")
	result, err := n.executeRestishCommand(api, command, filledArgs, body, prNumber, verbose)
	if err != nil {
		status.Done("")
		fmt.Fprintln(os.Stderr, formatAPIError(err.Error()))
		return errSilent
	}

	status.Done("")

	if result != "" && result != "null" {
		fmt.Println(formatResult(result))
	}

	n.updateConfigAfterCommand(api, command, result, sharedAPI.GetDeletedID(filledArgs, command))
	return nil
}

func (n *nexusCLI) confirmDestructiveAction(command string, args []string, filledFromConfig map[string]string, force bool) bool {
	if force {
		return true
	}
	if !isTerminal() {
		return true
	}

	isDelete := strings.HasPrefix(command, "delete-")
	isUpdate := strings.HasPrefix(command, "update-")

	// Delete commands always prompt for confirmation
	if isDelete {
		_, noun, _ := parseNounVerb(command)
		if noun == "" {
			noun = "resource"
		}
		if len(args) > 0 {
			fmt.Fprintf(os.Stderr, "About to delete %s: %s\n", noun, args[len(args)-1])
		}
		fmt.Fprintf(os.Stderr, "Are you sure you want to delete? [y/N]: ")

		var response string
		_, _ = fmt.Scanln(&response)
		response = strings.TrimSpace(strings.ToLower(response))
		return response == "y" || response == "yes"
	}

	// Update commands only prompt when args were auto-filled from config
	if isUpdate && len(filledFromConfig) > 0 {
		fmt.Fprintln(os.Stderr, "Using defaults from config:")
		for resourceType, id := range filledFromConfig {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", resourceType, id)
		}
		fmt.Fprintf(os.Stderr, "\nProceed with update? [y/N]: ")

		var response string
		_, _ = fmt.Scanln(&response)
		response = strings.TrimSpace(strings.ToLower(response))
		return response == "y" || response == "yes"
	}

	return true
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
			fmt.Fprintf(os.Stderr, "  Set a default with: astro context set %s <id>\n", resourceType)
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

	// Resolve variant (oneOf) before loading fields so we get the right schema.
	variantIndex := 0
	variants := schemaParser.GetVariants()
	if len(variants) > 0 {
		variantIndex = resolveVariant(variants, bodyFields)
	}

	fields := schemaParser.GetFields(variantIndex)
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

	if isTerminal() {
		status.Done("")
		promptForMissingFields(fields, bodyData)
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

// resolveVariant determines which oneOf variant to use.
// If the discriminator value is already in bodyFields, returns its variant index.
// If running in a terminal, prompts for selection. Otherwise defaults to 0.
func resolveVariant(variants []restish.SchemaVariant, bodyFields map[string]string) int {
	if len(variants) == 0 {
		return 0
	}

	discKey := variants[0].DiscriminatorKey
	if discKey == "" {
		return 0
	}

	// Check if discriminator was provided via --flags
	for k, v := range bodyFields {
		if strings.EqualFold(k, discKey) || strings.EqualFold(strcase.ToKebab(k), strcase.ToKebab(discKey)) {
			for _, variant := range variants {
				if strings.EqualFold(v, variant.Name) {
					return variant.Index
				}
			}
		}
	}

	if !isTerminal() {
		return 0
	}

	// Prompt for variant selection
	label := fieldFormatter.FormatSingleField(discKey)
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Fprintf(os.Stderr, "\n  %s:\n", label)
	for i, v := range variants {
		fmt.Fprintf(os.Stderr, "    %d) %s\n", i+1, v.Name)
	}
	for {
		fmt.Fprintf(os.Stderr, "  Select [1-%d]: ", len(variants))
		if !scanner.Scan() {
			return 0
		}
		input := strings.TrimSpace(scanner.Text())
		if idx, err := strconv.Atoi(input); err == nil && idx >= 1 && idx <= len(variants) {
			selected := variants[idx-1]
			bodyFields[discKey] = selected.Name
			return selected.Index
		}
	}
}

func promptForMissingFields(fields []restish.SchemaField, bodyData map[string]any) {
	scanner := bufio.NewScanner(os.Stdin)
	prompted := false

	for _, f := range fields {
		if !f.Required {
			continue
		}
		camelKey := strcase.ToLowerCamel(f.Name)
		if _, exists := bodyData[camelKey]; exists {
			continue
		}
		if f.Type == "object" || f.Type == "array" {
			continue
		}

		if !prompted {
			fmt.Fprintln(os.Stderr, "Missing required fields:")
			prompted = true
		}

		label := fieldFormatter.FormatSingleField(f.Name)

		if f.Enum != "" {
			options := strings.Split(f.Enum, ",")
			for i := range options {
				options[i] = strings.TrimSpace(options[i])
			}
			fmt.Fprintf(os.Stderr, "\n  %s:\n", label)
			for i, opt := range options {
				fmt.Fprintf(os.Stderr, "    %d) %s\n", i+1, opt)
			}
			for {
				fmt.Fprintf(os.Stderr, "  Select [1-%d]: ", len(options))
				if !scanner.Scan() {
					return
				}
				input := strings.TrimSpace(scanner.Text())
				if idx, err := strconv.Atoi(input); err == nil && idx >= 1 && idx <= len(options) {
					bodyData[camelKey] = options[idx-1]
					break
				}
			}
		} else {
			hint := f.Type
			if f.Description != "" {
				hint = f.Description
			}
			for {
				fmt.Fprintf(os.Stderr, "  %s (%s): ", label, hint)
				if !scanner.Scan() {
					return
				}
				input := strings.TrimSpace(scanner.Text())
				if input != "" {
					bodyData[camelKey] = convertToType(input, f.Type)
					break
				}
			}
		}
	}

	if prompted {
		fmt.Fprintln(os.Stderr)
	}
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

	noun, verb, ok := parseNounVerb(command)
	if ok {
		fmt.Println("Usage:")
		fmt.Printf("  astro %s %s [args...] [flags]\n", noun, verb)
	} else {
		fmt.Println("Usage:")
		fmt.Printf("  astro %s [args...] [flags]\n", command)
	}

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
