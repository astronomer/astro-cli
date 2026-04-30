package env

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"gopkg.in/yaml.v3"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

// Format selects the rendering for env-object output.
type Format string

const (
	FormatTable  Format = "table"
	FormatJSON   Format = "json"
	FormatYAML   Format = "yaml"
	FormatDotenv Format = "dotenv"
)

const maskedSecret = "****"

// ParseFormat parses a user-provided string into a Format. Empty string returns the zero value.
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case "":
		return "", nil
	case FormatTable, FormatJSON, FormatYAML, FormatDotenv:
		return Format(s), nil
	default:
		return "", fmt.Errorf("invalid format %q (want: table|json|yaml|dotenv)", s)
	}
}

// WriteVarList renders a list of ENVIRONMENT_VARIABLE objects in the requested format.
func WriteVarList(envObjs []astrocore.EnvironmentObject, format Format, includeSecrets bool, out io.Writer) error {
	switch format {
	case "", FormatTable:
		return writeVarTable(envObjs, includeSecrets, out)
	case FormatDotenv:
		return writeVarDotenv(envObjs, includeSecrets, out)
	case FormatJSON:
		return writeJSON(envObjs, out)
	case FormatYAML:
		return writeYAML(envObjs, out)
	}
	return fmt.Errorf("invalid format %q", format)
}

// WriteVar renders a single ENVIRONMENT_VARIABLE object.
func WriteVar(envObj *astrocore.EnvironmentObject, format Format, includeSecrets bool, out io.Writer) error {
	if envObj == nil {
		return errors.New("nil environment object")
	}
	switch format {
	case "", FormatTable:
		return writeVarTable([]astrocore.EnvironmentObject{*envObj}, includeSecrets, out)
	case FormatDotenv:
		return writeVarDotenv([]astrocore.EnvironmentObject{*envObj}, includeSecrets, out)
	case FormatJSON:
		return writeJSON(envObj, out)
	case FormatYAML:
		return writeYAML(envObj, out)
	}
	return fmt.Errorf("invalid format %q", format)
}

func writeVarTable(envObjs []astrocore.EnvironmentObject, includeSecrets bool, out io.Writer) error {
	if len(envObjs) == 0 {
		fmt.Fprintln(out, "No environment variables found")
		return nil
	}

	// The platform blanks out IDs when resolveLinked=true (the default for list/export),
	// so most callers see no IDs. Only show the ID column when at least one row has one.
	showID := false
	for i := range envObjs {
		if envObjs[i].Id != nil && *envObjs[i].Id != "" {
			showID = true
			break
		}
	}

	header := []string{"#", "KEY", "VALUE", "SECRET", "SCOPE"}
	if showID {
		header = append(header, "ID")
	}
	t := &printutil.Table{
		DynamicPadding: true,
		Header:         header,
	}
	for i := range envObjs {
		o := &envObjs[i]
		value := ""
		isSecret := false
		if o.EnvironmentVariable != nil {
			isSecret = o.EnvironmentVariable.IsSecret
			switch {
			case isSecret && !includeSecrets:
				value = maskedSecret
			default:
				value = o.EnvironmentVariable.Value
			}
		}
		row := []string{
			strconv.Itoa(i + 1),
			o.ObjectKey,
			value,
			strconv.FormatBool(isSecret),
			string(o.Scope),
		}
		if showID {
			id := ""
			if o.Id != nil {
				id = *o.Id
			}
			row = append(row, id)
		}
		t.AddRow(row, false)
	}
	t.Print(out)
	return nil
}

func writeVarDotenv(envObjs []astrocore.EnvironmentObject, includeSecrets bool, out io.Writer) error {
	for i := range envObjs {
		o := &envObjs[i]
		if o.EnvironmentVariable == nil {
			continue
		}
		isSecret := o.EnvironmentVariable.IsSecret
		switch {
		case isSecret && !includeSecrets:
			fmt.Fprintf(out, "%s=  # secret, use --include-secrets\n", o.ObjectKey)
		default:
			fmt.Fprintf(out, "%s=%s\n", o.ObjectKey, o.EnvironmentVariable.Value)
		}
	}
	return nil
}

func writeJSON(v any, out io.Writer) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeYAML(v any, out io.Writer) error {
	enc := yaml.NewEncoder(out)
	defer enc.Close()
	return enc.Encode(v)
}
