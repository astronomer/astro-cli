package env

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
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
	return writeLinksReport(report, format, func() linksTable { return varLinksTable(report, includeSecrets) }, out)
}

// writeLinksReport is the format switch both link reports share; only the
// table differs.
func writeLinksReport(report any, format Format, table func() linksTable, out io.Writer) error {
	switch format {
	case "", FormatTable:
		t := table()
		t.write(out)
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
	t.Print(out) //nolint:errcheck // best-effort render to the terminal
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

// linksTable is a link report as the table view shows it: a few labeled
// lines about the object, then its links, then its excludes. Both link
// reports render through it, so `variable link list` and `connection link
// list` read the same.
type linksTable struct {
	// fields are the labeled lines above the links, in order.
	fields [][2]string
	// overrideHeader names the links' override column.
	overrideHeader string
	// links are each link's deployment and rendered override.
	links    [][2]string
	excludes []string
}

func (t *linksTable) write(out io.Writer) {
	const label = "%-20s%s\n"
	for _, f := range t.fields {
		fmt.Fprintf(out, label, f[0]+":", f[1])
	}
	fmt.Fprintln(out)

	if len(t.links) == 0 {
		fmt.Fprintf(out, label, "LINKS:", "(none)")
	} else {
		linkTable := &printutil.Table{DynamicPadding: true, Header: []string{"#", "DEPLOYMENT", t.overrideHeader}}
		for i, l := range t.links {
			linkTable.AddRow([]string{strconv.Itoa(i + 1), l[0], clampTableValue(l[1])}, false)
		}
		fmt.Fprintln(out, "LINKS:")
		linkTable.Print(out) //nolint:errcheck // best-effort render to the terminal
	}

	fmt.Fprintln(out)
	if len(t.excludes) == 0 {
		fmt.Fprintf(out, label, "EXCLUDES:", "(none)")
	} else {
		fmt.Fprintln(out, "EXCLUDES:")
		for i, depID := range t.excludes {
			fmt.Fprintf(out, "  %d. %s\n", i+1, depID)
		}
	}
}

func varLinksTable(report *VarLinksReport, includeSecrets bool) linksTable {
	value := report.WorkspaceValue
	if report.IsSecret && !includeSecrets {
		value = maskedSecret + " (secret)"
	}
	t := linksTable{
		fields: [][2]string{
			{"KEY", report.ObjectKey},
			{"ID", report.ObjectID},
			{"WORKSPACE VALUE", clampTableValue(value)},
			{"AUTO-LINK", strconv.FormatBool(report.AutoLinkDeployments)},
		},
		overrideHeader: "OVERRIDE",
		excludes:       report.ExcludeLinks,
	}
	for _, l := range report.Links {
		t.links = append(t.links, [2]string{l.DeploymentID, overrideDisplay(l.OverrideValue, report.IsSecret, includeSecrets)})
	}
	return t
}

// WriteLinks renders a LinksReport in the requested format.
func WriteLinks(report *LinksReport, format Format, out io.Writer) error {
	return writeLinksReport(report, format, func() linksTable {
		t := linksTable{
			fields: [][2]string{
				{"KEY", report.ObjectKey},
				{"ID", report.ObjectID},
				{"AUTO-LINK", strconv.FormatBool(report.AutoLinkDeployments)},
			},
			overrideHeader: "OVERRIDES",
			excludes:       report.ExcludeLinks,
		}
		for _, l := range report.Links {
			t.links = append(t.links, [2]string{l.DeploymentID, overridesDisplay(l)})
		}
		return t
	}, out)
}

// overridesDisplay renders a link's overrides as name=value pairs. A field
// set but absent from the overrides is a secret the platform masked, shown
// as hidden rather than dropped, so a set password is not mistaken for none.
func overridesDisplay(l ObjectLink) string {
	var parts []string
	shown := map[string]bool{}
	for name, v := range l.Overrides {
		if extra, ok := v.(map[string]any); ok {
			for key, ev := range extra {
				parts = append(parts, fmt.Sprintf("%s.%s=%v", name, key, ev))
				shown[name+"."+key] = true
			}
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%v", name, v))
		shown[name] = true
	}
	for _, f := range l.SetFields {
		if !shown[f] {
			parts = append(parts, f+"=(hidden, use --include-secrets)")
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	slices.Sort(parts)
	return strings.Join(parts, " ")
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
	t.Print(out) //nolint:errcheck // best-effort render to the terminal
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
	t.Print(out) //nolint:errcheck // best-effort render to the terminal
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
	t.Print(out) //nolint:errcheck // best-effort render to the terminal
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
	defer enc.Close() //nolint:errcheck // best-effort close
	return enc.Encode(generic)
}

// WriteInventory renders the cross-kind listing.
//
// FormatDotenv is deliberately absent: a dotenv file is KEY=VALUE, and this
// listing has no values to put on the right of the equals sign. Accepting the
// flag and emitting keys with empty values would produce a file that, read
// back through `set --from-file`, is exactly the blank-every-secret shape that
// path now refuses.
func WriteInventory(items []InventoryItem, format Format, out io.Writer) error {
	switch format {
	case "", FormatTable:
		return writeInventoryTable(items, out)
	case FormatJSON:
		return writeJSON(items, out)
	case FormatYAML:
		return writeYAML(items, out)
	case FormatDotenv:
		return ErrInventoryHasNoValues
	}
	return fmt.Errorf("invalid format %q", format)
}

// ErrInventoryHasNoValues refuses a format that writes values for a listing
// that has none.
//
// It is a sentinel rather than a message built at each site because the check
// runs twice: once in the command, before paying for the objects, and once
// here, so a future caller cannot bypass it. Two hand-copied sentences would
// drift, and the early one is the one users see.
var ErrInventoryHasNoValues = errors.New(
	"that format writes values, which a cross-kind listing has none of; " +
		"use it on a single kind, e.g. `astro env variable export`")

func writeInventoryTable(items []InventoryItem, out io.Writer) error {
	if len(items) == 0 {
		fmt.Fprintln(out, "No environment objects found")
		return nil
	}
	header := []string{"#", "KIND", "KEY", "SCOPE"}
	showID := slices.ContainsFunc(items, func(i InventoryItem) bool { return i.ID != "" })
	if showID {
		header = append(header, "ID")
	}
	t := &printutil.Table{DynamicPadding: true, Header: header}
	for i := range items {
		row := []string{
			strconv.Itoa(i + 1),
			items[i].Kind,
			items[i].Key,
			items[i].Scope,
		}
		if showID {
			row = append(row, items[i].ID)
		}
		t.AddRow(row, false)
	}
	return t.Print(out)
}
