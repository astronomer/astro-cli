package env

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	astrov1 "github.com/astronomer/astro-cli/pkg/astro-client-v1"
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

const (
	maskedSecret = "****"

	// tableValueMax caps the width of value-style columns in the table view.
	// JSON/YAML/dotenv output is never truncated; this is a display concession
	// so a single 5000-char value doesn't shred the layout.
	tableValueMax = 60
)

// clampTableValue prepares a string for a single table cell: collapses
// newlines to a visual marker and truncates over tableValueMax runes.
func clampTableValue(s string) string {
	if s == "" {
		return ""
	}
	if strings.ContainsAny(s, "\r\n") {
		s = strings.NewReplacer("\r\n", " ⏎ ", "\n", " ⏎ ", "\r", " ⏎ ").Replace(s)
	}
	r := []rune(s)
	if len(r) > tableValueMax {
		return string(r[:tableValueMax-1]) + "…"
	}
	return s
}

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
func WriteVarList(envObjs []astrov1.EnvironmentObject, format Format, includeSecrets bool, out io.Writer) error {
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
func WriteVar(envObj *astrov1.EnvironmentObject, format Format, includeSecrets bool, out io.Writer) error {
	if envObj == nil {
		return errors.New("nil environment object")
	}
	switch format {
	case "", FormatTable:
		return writeVarTable([]astrov1.EnvironmentObject{*envObj}, includeSecrets, out)
	case FormatDotenv:
		return writeVarDotenv([]astrov1.EnvironmentObject{*envObj}, includeSecrets, out)
	case FormatJSON:
		return writeJSON(envObj, out)
	case FormatYAML:
		return writeYAML(envObj, out)
	}
	return fmt.Errorf("invalid format %q", format)
}

// WriteVarLinks renders a VarLinksReport in the requested format.
func WriteVarLinks(report *VarLinksReport, format Format, includeSecrets bool, out io.Writer) error {
	switch format {
	case "", FormatTable:
		writeVarLinksTable(report, includeSecrets, out)
		return nil
	case FormatJSON:
		return writeJSON(report, out)
	case FormatYAML:
		return writeYAML(report, out)
	case FormatDotenv:
		return errors.New("dotenv format is not supported for links")
	}
	return fmt.Errorf("invalid format %q", format)
}

// WriteConnList renders a list of CONNECTION objects.
func WriteConnList(envObjs []astrov1.EnvironmentObject, format Format, out io.Writer) error {
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
func WriteConn(envObj *astrov1.EnvironmentObject, format Format, out io.Writer) error {
	if envObj == nil {
		return errors.New("nil environment object")
	}
	if format == FormatJSON {
		return writeJSON(envObj, out)
	}
	if format == FormatYAML {
		return writeYAML(envObj, out)
	}
	return writeConnTable([]astrov1.EnvironmentObject{*envObj}, out)
}

// WriteAirflowVarList renders a list of AIRFLOW_VARIABLE objects.
// Same shape as ENVIRONMENT_VARIABLE.
func WriteAirflowVarList(envObjs []astrov1.EnvironmentObject, format Format, includeSecrets bool, out io.Writer) error {
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
func WriteAirflowVar(envObj *astrov1.EnvironmentObject, format Format, includeSecrets bool, out io.Writer) error {
	if envObj == nil {
		return errors.New("nil environment object")
	}
	if format == FormatJSON {
		return writeJSON(envObj, out)
	}
	if format == FormatYAML {
		return writeYAML(envObj, out)
	}
	return writeAirflowVarTable([]astrov1.EnvironmentObject{*envObj}, includeSecrets, out)
}

// WriteMetricsExportList renders a list of METRICS_EXPORT objects.
func WriteMetricsExportList(envObjs []astrov1.EnvironmentObject, format Format, out io.Writer) error {
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
func WriteMetricsExport(envObj *astrov1.EnvironmentObject, format Format, out io.Writer) error {
	if envObj == nil {
		return errors.New("nil environment object")
	}
	if format == FormatJSON {
		return writeJSON(envObj, out)
	}
	if format == FormatYAML {
		return writeYAML(envObj, out)
	}
	return writeMetricsExportTable([]astrov1.EnvironmentObject{*envObj}, out)
}

func writeVarTable(envObjs []astrov1.EnvironmentObject, includeSecrets bool, out io.Writer) error {
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
			clampTableValue(value),
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

func writeVarDotenv(envObjs []astrov1.EnvironmentObject, includeSecrets bool, out io.Writer) error {
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
			fmt.Fprintf(out, "%s=%s\n", o.ObjectKey, dotenvQuote(o.EnvironmentVariable.Value))
		}
	}
	return nil
}

// dotenvEscaper escapes characters that are special inside a double-quoted
// dotenv value: `\`, `"`, `$` (which would otherwise be variable-expanded by
// most parsers), and the standard whitespace escapes.
var dotenvEscaper = strings.NewReplacer(
	`\`, `\\`,
	`"`, `\"`,
	`$`, `\$`,
	"\n", `\n`,
	"\r", `\r`,
	"\t", `\t`,
)

// dotenvQuote renders a value safely for a dotenv line. Bare values are emitted
// unquoted; values that contain whitespace, newlines, comment markers, quotes,
// `$` (variable-expansion trigger), `\`, or a leading `export ` are wrapped in
// double quotes with the special characters escaped. This matches what godotenv,
// python-dotenv, and dotenvx accept and round-trip cleanly.
func dotenvQuote(v string) string {
	if v == "" {
		return ""
	}
	if !strings.ContainsAny(v, " \t\r\n\"'#\\$") && !strings.HasPrefix(v, "export ") {
		return v
	}
	return `"` + dotenvEscaper.Replace(v) + `"`
}

// overrideDisplay renders a per-link override value for the table view, with a
// special "(hidden)" marker for secret vars when --include-secrets is off
// (in which case the platform redacts the override and we can't distinguish
// "no override" from "redacted override"; flag the ambiguity instead of
// quietly showing "-").
func overrideDisplay(override *string, isSecret, includeSecrets bool) string {
	if isSecret && !includeSecrets {
		return "(hidden, use --include-secrets)"
	}
	if override == nil {
		return "-"
	}
	return *override
}

func writeVarLinksTable(report *VarLinksReport, includeSecrets bool, out io.Writer) {
	value := report.WorkspaceValue
	if report.IsSecret && !includeSecrets {
		value = maskedSecret + " (secret)"
	}
	fmt.Fprintf(out, "KEY:                %s\n", report.ObjectKey)
	fmt.Fprintf(out, "ID:                 %s\n", report.ObjectID)
	fmt.Fprintf(out, "WORKSPACE VALUE:    %s\n", clampTableValue(value))
	fmt.Fprintf(out, "AUTO-LINK:          %t\n", report.AutoLinkDeployments)
	fmt.Fprintln(out)

	if len(report.Links) == 0 {
		fmt.Fprintln(out, "LINKS:              (none)")
	} else {
		linkTable := &printutil.Table{DynamicPadding: true, Header: []string{"#", "DEPLOYMENT", "OVERRIDE"}}
		for i, l := range report.Links {
			override := clampTableValue(overrideDisplay(l.OverrideValue, report.IsSecret, includeSecrets))
			linkTable.AddRow([]string{strconv.Itoa(i + 1), l.DeploymentID, override}, false)
		}
		fmt.Fprintln(out, "LINKS:")
		linkTable.Print(out)
	}

	fmt.Fprintln(out)
	if len(report.ExcludeLinks) == 0 {
		fmt.Fprintln(out, "EXCLUDES:           (none)")
	} else {
		fmt.Fprintln(out, "EXCLUDES:")
		for i, depID := range report.ExcludeLinks {
			fmt.Fprintf(out, "  %d. %s\n", i+1, depID)
		}
	}
}

func writeConnTable(envObjs []astrov1.EnvironmentObject, out io.Writer) error {
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
		row := []string{strconv.Itoa(i + 1), o.ObjectKey, connType, clampTableValue(host), port, clampTableValue(login), schema, string(o.Scope)}
		if showID {
			row = append(row, ptrStr(o.Id))
		}
		t.AddRow(row, false)
	}
	t.Print(out)
	return nil
}

func writeAirflowVarTable(envObjs []astrov1.EnvironmentObject, includeSecrets bool, out io.Writer) error {
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
		row := []string{strconv.Itoa(i + 1), o.ObjectKey, clampTableValue(value), strconv.FormatBool(isSecret), string(o.Scope)}
		if showID {
			row = append(row, ptrStr(o.Id))
		}
		t.AddRow(row, false)
	}
	t.Print(out)
	return nil
}

func writeMetricsExportTable(envObjs []astrov1.EnvironmentObject, out io.Writer) error {
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
		row := []string{strconv.Itoa(i + 1), o.ObjectKey, exporter, clampTableValue(endpoint), authType, string(o.Scope)}
		if showID {
			row = append(row, ptrStr(o.Id))
		}
		t.AddRow(row, false)
	}
	t.Print(out)
	return nil
}

func anyHasID(envObjs []astrov1.EnvironmentObject) bool {
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
	// The generated env-object types only carry JSON tags, so a direct YAML
	// encode would emit lowercased field names. Round-trip through JSON to
	// preserve camelCase keys consistently with --format json.
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var generic any
	if err := json.Unmarshal(jsonBytes, &generic); err != nil {
		return err
	}
	enc := yaml.NewEncoder(out)
	defer enc.Close()
	return enc.Encode(generic)
}
