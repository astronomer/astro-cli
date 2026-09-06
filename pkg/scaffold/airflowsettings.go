package scaffold

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/connmodel"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

// SettingsRelPath is the v1 file this transform reads.
const SettingsRelPath = "airflow_settings.yaml"

// A v1 project declares its Airflow connections, Variables and pools in
// airflow_settings.yaml, in cleartext, in a file that is normally committed.
// v2 splits that content by what it is rather than where it came from.
//
// # Connections go to the vault, and are declared
//
// A connection is a credential. Its value goes to the shared vault at the
// project's scope — the same place `astro local env set --secret` writes and
// `astro local start` resolves — and the manifest gets a declaration naming it
// and its conn_type, marked sensitive.
//
// The declaration carries no `optional`, so it is required. That is deliberate
// and it is the whole security improvement: for whoever converts, the vault
// write satisfies it immediately; for a teammate who pulls the converted repo,
// the connection is missing and the project says which one and refuses to
// start, instead of quietly running against the author's committed password.
//
// # Variables stay in the manifest
//
// An Airflow Variable in this file is committed config with no credential
// semantics, and the file offers no way to mark one secret. It is carried as a
// declared default, which keeps it exactly as shared as it is today.
//
// Routing them to the vault instead would read as the safer choice and is the
// more destructive one: the value would leave the repository, every teammate's
// converted project would have a required declaration with nothing behind it,
// and a Variable that was never a secret would have become a startup failure on
// every machine but one.
//
// # Pools do not move
//
// Neither tool persists pools — v2 has nowhere to put them and v1 only replayed
// them into a running Airflow — so they are reported and the file is kept.

// carriedSettings is what airflow_settings.yaml yielded.
//
// It mirrors carriedEnvSchema, and for the same all-or-nothing reason: writing
// SOME of this file's declarations creates [tool.astro.env], which makes the
// manifest the project's declaration source from then on, so whatever stayed
// behind in the YAML has silently stopped applying. See that type's doc.
type carriedSettings struct {
	// schema is the declarations to merge into [tool.astro.env].
	schema *envschema.Schema
	// secrets are the vault writes, in name order. Values live here and never
	// in a Change, so nothing that previews or serializes a changeset can carry
	// a credential.
	secrets []SecretWrite
	// pools names the pools found, so the note can say which ones stay behind.
	pools []string

	blockers   []string
	advisories []string
}

// settingsDoc is the v1 file's shape. Deliberately a local type rather than a
// borrow from the root module's settings package: that one is built on viper
// and drags in docker and the platform clients, and this module is a leaf.
type settingsDoc struct {
	Airflow struct {
		Connections []settingsConn `yaml:"connections"`
		Variables   []settingsVar  `yaml:"variables"`
		Pools       []settingsPool `yaml:"pools"`
	} `yaml:"airflow"`
}

// settingsConn holds conn_port and conn_extra as `any` rather than their real
// types, and that is not laziness.
//
// v1 read this file through viper, which coerces: `conn_port: "5432"` was an
// int and `conn_extra: '{"sslmode":"require"}'` was a map, because Airflow's
// own extra is a JSON string and people write it that way. yaml.v3 decoding
// into a typed struct refuses both — and refuses them by failing the decode of
// the WHOLE document, so one quoted port in one connection carried nothing from
// a file `astro dev start` read without complaint.
//
// Decoding permissively and coercing per entry keeps a bad scalar a fault of
// the entry it is in, which is what "report every reason" needs.
type settingsConn struct {
	ConnID       string `yaml:"conn_id"`
	ConnType     string `yaml:"conn_type"`
	ConnHost     string `yaml:"conn_host"`
	ConnSchema   string `yaml:"conn_schema"`
	ConnLogin    string `yaml:"conn_login"`
	ConnPassword string `yaml:"conn_password"`
	ConnPort     any    `yaml:"conn_port"`
	ConnURI      string `yaml:"conn_uri"`
	ConnExtra    any    `yaml:"conn_extra"`
}

type settingsVar struct {
	Name  string `yaml:"variable_name"`
	Value string `yaml:"variable_value"`
}

type settingsPool struct {
	Name string `yaml:"pool_name"`
}

// readAirflowSettings decides what the file can carry, and reports every reason
// it cannot — every reason rather than the first, because this feeds a preview
// of a destructive operation and a user who fixes what it names should not meet
// a second fault on the next run.
//
// A file that will not parse is a blocker, not an error: failing the run would
// leave a user unable to convert until they hand-fixed a file by themselves.
func readAirflowSettings(data []byte) carriedSettings {
	var doc settingsDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return carriedSettings{blockers: []string{
			SettingsRelPath + " was not carried, and is kept as it is. " + err.Error(),
		}}
	}

	out := carriedSettings{schema: &envschema.Schema{
		AirflowVariables: map[string]envschema.ValueSpec{},
		Connections:      map[string]envschema.ValueSpec{},
	}}
	out.readConnections(doc.Airflow.Connections)
	out.readVariables(doc.Airflow.Variables)
	for _, p := range doc.Airflow.Pools {
		if strings.TrimSpace(p.Name) != "" {
			out.pools = append(out.pools, p.Name)
		}
	}

	if len(out.blockers) > 0 {
		return carriedSettings{blockers: out.blockers, pools: out.pools}
	}
	return out
}

func (c *carriedSettings) readConnections(conns []settingsConn) {
	// Keyed by the UPPERCASED id. A connection resolves through
	// AIRFLOW_CONN_<ID>, which airflowenv uppercases, so "Warehouse" and
	// "warehouse" are two entries here and one variable at start — carrying
	// both means the project runs against whichever won by composition order,
	// with nothing said about it.
	seen := map[string]string{}
	for i := range conns {
		sc := &conns[i]
		id := strings.TrimSpace(sc.ConnID)
		if id == "" {
			c.blockers = append(c.blockers, SettingsRelPath+": a connection has no conn_id")
			continue
		}
		if first, dup := seen[strings.ToUpper(id)]; dup {
			c.blockers = append(c.blockers, SettingsRelPath+": "+id+" and "+first+
				" are the same connection to Airflow, which reads both as AIRFLOW_CONN_"+strings.ToUpper(id))
			continue
		}
		seen[strings.ToUpper(id)] = id

		if err := envschema.CheckName(envschema.SectionConnection, id); err != nil {
			c.blockers = append(c.blockers, SettingsRelPath+": "+err.Error())
			continue
		}
		// The vault key is the other name this has to be legal as, and it has
		// its own rules. Checking here rather than at the write means a bad name
		// blocks the carry instead of failing an Apply halfway through.
		if _, err := secrets.Key(secrets.KindConn, secrets.GlobalScope, id); err != nil {
			c.blockers = append(c.blockers, SettingsRelPath+": "+id+" cannot be a vault key. "+err.Error())
			continue
		}

		spec := envschema.ValueSpec{ConnType: strings.TrimSpace(sc.ConnType), Sensitive: true}
		if problems := spec.Check(envschema.SectionConnection); len(problems) > 0 {
			for _, p := range problems {
				c.blockers = append(c.blockers, SettingsRelPath+": "+id+" cannot be carried. "+p.Reason)
			}
			continue
		}
		c.schema.Connections[id] = spec

		value, err := connValue(sc)
		if err != nil {
			c.blockers = append(c.blockers, SettingsRelPath+": "+id+" cannot be carried. "+err.Error())
			continue
		}
		if value == "" {
			// Nothing to store. The declaration still goes in, so the project
			// says the connection is expected and refuses to start until it is
			// set — which is what an empty entry in the v1 file amounted to
			// anyway, minus the part where nothing said so.
			c.advisories = append(c.advisories, id+
				": declared as a required connection, with no value to carry. "+
				"Set it with `astro local env set --secret`")
			continue
		}
		c.secrets = append(c.secrets, SecretWrite{
			Kind:  secrets.KindConn,
			Name:  id,
			Label: "connection " + id,
			value: value,
		})
	}
}

func (c *carriedSettings) readVariables(vars []settingsVar) {
	// Uppercased for the same reason as a conn_id: a Variable reaches Airflow
	// as AIRFLOW_VAR_<KEY>.
	seen := map[string]string{}
	for _, v := range vars {
		name := strings.TrimSpace(v.Name)
		if name == "" {
			c.blockers = append(c.blockers, SettingsRelPath+": a variable has no variable_name")
			continue
		}
		if first, dup := seen[strings.ToUpper(name)]; dup {
			c.blockers = append(c.blockers, SettingsRelPath+": "+name+" and "+first+
				" are the same variable to Airflow, which reads both as AIRFLOW_VAR_"+strings.ToUpper(name))
			continue
		}
		seen[strings.ToUpper(name)] = name

		if err := envschema.CheckName(envschema.SectionAirflowVariable, name); err != nil {
			c.blockers = append(c.blockers, SettingsRelPath+": "+err.Error())
			continue
		}
		spec := envschema.ValueSpec{Default: v.Value, HasDefault: true}
		if problems := spec.Check(envschema.SectionAirflowVariable); len(problems) > 0 {
			for _, p := range problems {
				c.blockers = append(c.blockers, SettingsRelPath+": "+name+" cannot be carried. "+p.Reason)
			}
			continue
		}
		c.schema.AirflowVariables[name] = spec
	}
}

// connValue is the vault payload for one connection: the canonical JSON, or
// the empty string when the entry supplied nothing to store.
//
// Both arms end at airflowenv, which is the one definition of what a stored
// connection looks like. A conn_uri is normalized through the same function
// `astro local env set --secret` uses, rather than stored as written: the vault
// holds JSON, and a URI sitting in it is a record only one of the two tools can
// read back.
//
// conn_uri wins over the broken-out fields when both are set, because that is
// how the v1 file spells a connection whose parts it did not enumerate — and
// merging the two would invent a connection the user never wrote.
func connValue(sc *settingsConn) (string, error) {
	if uri := strings.TrimSpace(sc.ConnURI); uri != "" {
		return airflowenv.NormalizeConn(strings.TrimSpace(sc.ConnID), uri)
	}
	port, err := connPort(sc.ConnPort)
	if err != nil {
		return "", err
	}
	extra, err := connExtra(sc.ConnExtra)
	if err != nil {
		return "", err
	}
	c := connmodel.Connection{
		ConnID:       strings.TrimSpace(sc.ConnID),
		ConnType:     strings.TrimSpace(sc.ConnType),
		ConnHost:     sc.ConnHost,
		ConnSchema:   sc.ConnSchema,
		ConnLogin:    sc.ConnLogin,
		ConnPassword: sc.ConnPassword,
		ConnPort:     port,
		ConnExtra:    extra,
	}
	if !hasConnValue(&c) {
		return "", nil
	}
	_, encoded, ok := airflowenv.EncodeConnEnv(c)
	if !ok {
		return "", fmt.Errorf("connection %q: could not encode value", c.ConnID)
	}
	return encoded, nil
}

// connPort accepts what v1 accepted: an integer, or a string holding one.
func connPort(v any) (int, error) {
	switch p := v.(type) {
	case nil:
		return 0, nil
	case int:
		return p, nil
	case string:
		if strings.TrimSpace(p) == "" {
			return 0, nil
		}
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return 0, fmt.Errorf("conn_port %q is not a number", p)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("conn_port must be a number, not %T", v)
	}
}

// connExtra accepts what v1 accepted: a mapping, or a string holding the JSON
// object Airflow itself stores an extra as.
func connExtra(v any) (map[string]any, error) {
	switch e := v.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		return e, nil
	case string:
		if strings.TrimSpace(e) == "" {
			return nil, nil
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(e), &out); err != nil {
			return nil, fmt.Errorf("conn_extra is neither a mapping nor JSON: %w", err)
		}
		return out, nil
	case map[any]any:
		// yaml.v3 yields this for a nested mapping with non-string keys.
		out := make(map[string]any, len(e))
		for k, val := range e {
			ks, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("conn_extra has a non-text key %v", k)
			}
			out[ks] = val
		}
		return out, nil
	default:
		return nil, fmt.Errorf("conn_extra must be a mapping or JSON text, not %T", v)
	}
}

// hasConnValue reports whether anything but the identity was filled in. A
// conn_type alone is a declaration, not a value: it is already carried as the
// declaration's conn_type, and storing it would satisfy a required connection
// with a record that configures nothing.
func hasConnValue(c *connmodel.Connection) bool {
	return c.ConnHost != "" || c.ConnSchema != "" || c.ConnLogin != "" ||
		c.ConnPassword != "" || c.ConnPort != 0 || len(c.ConnExtra) > 0
}

// declares reports whether anything survived to be written.
func (c *carriedSettings) declares() bool {
	if c.schema == nil {
		return false
	}
	return len(c.schema.AirflowVariables)+len(c.schema.Connections) > 0
}

// notes is what stays behind: the pools, which have nowhere to go, and so the
// reason the file is kept rather than retired.
func (c *carriedSettings) notes() []string {
	if len(c.pools) == 0 {
		return nil
	}
	sorted := append([]string(nil), c.pools...)
	sort.Strings(sorted)
	return []string{fmt.Sprintf(
		"%s: %s kept for its pools (%s). Neither `astro local start` nor the app stores pools, so set them in Airflow",
		SettingsRelPath, plural(len(sorted), "pool", "pools"), strings.Join(sorted, ", "),
	)}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// migratedFrom names the files that fed [tool.astro.env], so the preview line
// says where the declarations came from rather than naming whichever file the
// feature was built for first.
func migratedFrom(v1 *v1Project) string {
	var from []string
	if v1.settings.declares() {
		from = append(from, SettingsRelPath)
	}
	if len(from) == 0 || hasEnvSchemaDeclarations(v1) {
		from = append([]string{envschema.LegacyRelPath}, from...)
	}
	return strings.Join(from, " and ")
}

// hasEnvSchemaDeclarations reports whether the env-schema FILE contributed,
// as opposed to the merged result it shares with airflow_settings.yaml.
func hasEnvSchemaDeclarations(v1 *v1Project) bool {
	if v1.envSchema.schema == nil {
		return false
	}
	// EnvVars can only have come from .astro/env.schema.yaml — this transform
	// declares none — so it settles the question on its own. The other two
	// sections are shared, and the merge already refused a name both files
	// declare, so anything the settings file did not contribute is the schema's.
	if len(v1.envSchema.schema.EnvVars) > 0 {
		return true
	}
	settings := v1.settings.schema
	if settings == nil {
		return true
	}
	return len(v1.envSchema.schema.AirflowVariables) > len(settings.AirflowVariables) ||
		len(v1.envSchema.schema.Connections) > len(settings.Connections)
}
