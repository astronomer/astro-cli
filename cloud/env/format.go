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

// WriteConnList renders a list of CONNECTION objects.
func WriteConnList(envObjs []astrocore.EnvironmentObject, format Format, out io.Writer) error {
	switch format {
	case "", FormatTable:
		return writeConnTable(envObjs, out)
	case FormatJSON:
		return writeJSON(envObjs, out)
	case FormatYAML:
		return writeYAML(envObjs, out)
	case FormatDotenv:
		return errors.New("dotenv format is not supported for connections")
	}
	return fmt.Errorf("invalid format %q", format)
}

// WriteConn renders a single CONNECTION object.
func WriteConn(envObj *astrocore.EnvironmentObject, format Format, out io.Writer) error {
	if envObj == nil {
		return errors.New("nil environment object")
	}
	if format == FormatJSON {
		return writeJSON(envObj, out)
	}
	if format == FormatYAML {
		return writeYAML(envObj, out)
	}
	return writeConnTable([]astrocore.EnvironmentObject{*envObj}, out)
}

// WriteAirflowVarList renders a list of AIRFLOW_VARIABLE objects.
// Same shape as ENVIRONMENT_VARIABLE.
func WriteAirflowVarList(envObjs []astrocore.EnvironmentObject, format Format, includeSecrets bool, out io.Writer) error {
	switch format {
	case "", FormatTable:
		return writeAirflowVarTable(envObjs, includeSecrets, out)
	case FormatJSON:
		return writeJSON(envObjs, out)
	case FormatYAML:
		return writeYAML(envObjs, out)
	case FormatDotenv:
		return errors.New("dotenv format is not supported for Airflow variables")
	}
	return fmt.Errorf("invalid format %q", format)
}

// WriteAirflowVar renders a single AIRFLOW_VARIABLE object.
func WriteAirflowVar(envObj *astrocore.EnvironmentObject, format Format, includeSecrets bool, out io.Writer) error {
	if envObj == nil {
		return errors.New("nil environment object")
	}
	if format == FormatJSON {
		return writeJSON(envObj, out)
	}
	if format == FormatYAML {
		return writeYAML(envObj, out)
	}
	return writeAirflowVarTable([]astrocore.EnvironmentObject{*envObj}, includeSecrets, out)
}

// WriteMetricsExportList renders a list of METRICS_EXPORT objects.
func WriteMetricsExportList(envObjs []astrocore.EnvironmentObject, format Format, out io.Writer) error {
	switch format {
	case "", FormatTable:
		return writeMetricsExportTable(envObjs, out)
	case FormatJSON:
		return writeJSON(envObjs, out)
	case FormatYAML:
		return writeYAML(envObjs, out)
	case FormatDotenv:
		return errors.New("dotenv format is not supported for metrics exports")
	}
	return fmt.Errorf("invalid format %q", format)
}

// WriteMetricsExport renders a single METRICS_EXPORT object.
func WriteMetricsExport(envObj *astrocore.EnvironmentObject, format Format, out io.Writer) error {
	if envObj == nil {
		return errors.New("nil environment object")
	}
	if format == FormatJSON {
		return writeJSON(envObj, out)
	}
	if format == FormatYAML {
		return writeYAML(envObj, out)
	}
	return writeMetricsExportTable([]astrocore.EnvironmentObject{*envObj}, out)
}

func writeVarTable(envObjs []astrocore.EnvironmentObject, includeSecrets bool, out io.Writer) error {
	if len(envObjs) == 0 {
		fmt.Fprintln(out, "No environment variables found")
		return nil
	}

	// The platform blanks out IDs when resolveLinked=true (the default for list/export),
	// so most callers see no IDs. Only show the ID column when at least one row has one.
	showID := anyHasID(envObjs)
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

func writeConnTable(envObjs []astrocore.EnvironmentObject, out io.Writer) error {
	if len(envObjs) == 0 {
		fmt.Fprintln(out, "No connections found")
		return nil
	}
	header := []string{"#", "KEY", "TYPE", "HOST", "PORT", "LOGIN", "SCHEMA", "SCOPE"}
	showID := anyHasID(envObjs)
	if showID {
		header = append(header, "ID")
	}
	t := &printutil.Table{DynamicPadding: true, Header: header}
	for i := range envObjs {
		o := &envObjs[i]
		var connType, host, port, login, schema string
		if o.Connection != nil {
			connType = o.Connection.Type
			host = ptrStr(o.Connection.Host)
			login = ptrStr(o.Connection.Login)
			schema = ptrStr(o.Connection.Schema)
			if o.Connection.Port != nil {
				port = strconv.Itoa(*o.Connection.Port)
			}
		}
		row := []string{strconv.Itoa(i + 1), o.ObjectKey, connType, host, port, login, schema, string(o.Scope)}
		if showID {
			row = append(row, ptrStr(o.Id))
		}
		t.AddRow(row, false)
	}
	t.Print(out)
	return nil
}

func writeAirflowVarTable(envObjs []astrocore.EnvironmentObject, includeSecrets bool, out io.Writer) error {
	if len(envObjs) == 0 {
		fmt.Fprintln(out, "No Airflow variables found")
		return nil
	}
	header := []string{"#", "KEY", "VALUE", "SECRET", "SCOPE"}
	showID := anyHasID(envObjs)
	if showID {
		header = append(header, "ID")
	}
	t := &printutil.Table{DynamicPadding: true, Header: header}
	for i := range envObjs {
		o := &envObjs[i]
		value := ""
		isSecret := false
		if o.AirflowVariable != nil {
			isSecret = o.AirflowVariable.IsSecret
			switch {
			case isSecret && !includeSecrets:
				value = maskedSecret
			default:
				value = o.AirflowVariable.Value
			}
		}
		row := []string{strconv.Itoa(i + 1), o.ObjectKey, value, strconv.FormatBool(isSecret), string(o.Scope)}
		if showID {
			row = append(row, ptrStr(o.Id))
		}
		t.AddRow(row, false)
	}
	t.Print(out)
	return nil
}

func writeMetricsExportTable(envObjs []astrocore.EnvironmentObject, out io.Writer) error {
	if len(envObjs) == 0 {
		fmt.Fprintln(out, "No metrics exports found")
		return nil
	}
	header := []string{"#", "KEY", "EXPORTER", "ENDPOINT", "AUTH", "SCOPE"}
	showID := anyHasID(envObjs)
	if showID {
		header = append(header, "ID")
	}
	t := &printutil.Table{DynamicPadding: true, Header: header}
	for i := range envObjs {
		o := &envObjs[i]
		var exporter, endpoint, authType string
		if o.MetricsExport != nil {
			exporter = string(o.MetricsExport.ExporterType)
			endpoint = o.MetricsExport.Endpoint
			if o.MetricsExport.AuthType != nil {
				authType = string(*o.MetricsExport.AuthType)
			}
		}
		row := []string{strconv.Itoa(i + 1), o.ObjectKey, exporter, endpoint, authType, string(o.Scope)}
		if showID {
			row = append(row, ptrStr(o.Id))
		}
		t.AddRow(row, false)
	}
	t.Print(out)
	return nil
}

func anyHasID(envObjs []astrocore.EnvironmentObject) bool {
	for i := range envObjs {
		if envObjs[i].Id != nil && *envObjs[i].Id != "" {
			return true
		}
	}
	return false
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
