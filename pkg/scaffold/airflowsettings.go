package scaffold

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/connmodel"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

// SettingsRelPath is the v1 file this transform reads.
const SettingsRelPath = "airflow_settings.yaml"

// A v1 project declares its Airflow connections, Variables and pools in
// airflow_settings.yaml, in cleartext, in a file v1 kept out of version control.
// v2 splits that content by what it is rather than where it came from.
//
// # Connections go to the vault, and are declared
//
// A connection is a credential. Its value goes to the shared vault at the
// project's scope — the same place `astro local env connection set --secret` writes and
// `astro local start` resolves — and the manifest gets a declaration naming it
// and its conn_type, marked sensitive.
//
// The declaration carries no `optional`, so it is required. That is deliberate
// and it is the whole security improvement: for whoever converts, the vault
// write satisfies it immediately; for a teammate who pulls the converted repo,
// the connection is missing and the project says which one and refuses to
// start, instead of quietly running against the author's committed password.
//
// # Variables go to the vault too
//
// An Airflow Variable's value takes the same path as a connection's: the vault
// at the project's scope, and a declaration in the manifest that carries no
// value and is marked sensitive. The v1 file offers no way to mark a Variable
// secret, it routinely holds API tokens, and v1's project template kept the
// file out of version control — so a declared default would commit tokens that
// were never committed before. Every one is marked sensitive rather than
// guessed at from its name, so the declaration says where the value lives and
// a later `set --secret=false` cannot move it into a plain file.
//
// # Pools go to [tool.astro.pools]
//
// A pool holds no secret, so it goes into the manifest as written: its name,
// its slots and its description. `astro local start` creates or updates each
// one, which is what v1 did with this file on every start. A file whose pools
// and values are all carried is retired.

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
	// pools is what goes to [tool.astro.pools]. It is read apart from the rest
	// of the file, both ways: a pool declares nothing in [tool.astro.env], so
	// the all-or-nothing reason above does not reach it, and a pool that
	// cannot be carried does not stop the connections and variables.
	pools carriedPools
	// held names the carried values the project already holds a different
	// value for. Apply does not write over them, so the file stays as the only
	// copy of its own values.
	held []string
	// heldIn is where each held name's copy is, for the advisory.
	heldIn map[string]SecretHeld
	// unstored reports that the values stay only in the file: the caller gave
	// no writer, or the vault could not be asked.
	unstored bool

	blockers   []string
	advisories []string
}

// carriedPools is what the file's pools yielded.
type carriedPools struct {
	byName map[string]manifest.Pool
	// notes name each entry that was not carried. They keep the file, as the
	// only record of that entry.
	notes      []string
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
	Name        string `yaml:"pool_name"`
	Slot        any    `yaml:"pool_slot"`
	Description string `yaml:"pool_description"`
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
	out.pools = readPools(doc.Airflow.Pools)

	if len(out.blockers) > 0 {
		return carriedSettings{blockers: out.blockers, pools: out.pools}
	}
	return out
}

// readPools carries each pool Airflow can take. v1 skipped a pool it could not
// create, with a line saying so; here the entry is named in a note instead,
// and the file stays.
func readPools(pools []settingsPool) carriedPools {
	var c carriedPools
	for i := range pools {
		p := &pools[i]
		name := strings.TrimSpace(p.Name)
		if name == "" {
			if !blankPool(p) {
				c.notes = append(c.notes, SettingsRelPath+": pool "+strconv.Itoa(i+1)+
					" has no pool_name. Add one, or delete the entry")
			}
			continue
		}
		if _, dup := c.byName[name]; dup {
			c.notes = append(c.notes, SettingsRelPath+": pool "+name+" is listed twice, and only the first was carried. Delete one entry")
			continue
		}
		if reason := manifest.PoolNameProblem(name); reason != "" {
			c.notes = append(c.notes, SettingsRelPath+": pool "+name+" cannot be carried: "+reason)
			continue
		}
		slots, err := poolSlot(p.Slot)
		if err != nil {
			c.notes = append(c.notes, SettingsRelPath+": pool "+name+" cannot be carried. "+err.Error())
			continue
		}
		pool := manifest.Pool{Slots: slots, Description: strings.TrimSpace(p.Description)}
		if name == manifest.DefaultPoolName && pool.Description != "" {
			// Airflow 3 refuses to change default_pool's description, so the
			// manifest refuses to state one. Its slots are what matter.
			pool.Description = ""
			c.advisories = append(c.advisories, SettingsRelPath+": carried default_pool's slots and not its description, "+
				"which Airflow does not let a caller change")
		}
		if c.byName == nil {
			c.byName = map[string]manifest.Pool{}
		}
		c.byName[name] = pool
	}
	return c
}

// poolSlot accepts what v1 accepted, an integer or a string holding one, and
// what Airflow accepts: a number above zero, or -1 for no limit.
func poolSlot(v any) (int, error) {
	var n int
	switch s := v.(type) {
	case nil:
		return 0, errors.New("pool_slot is missing")
	case int:
		n = s
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return 0, fmt.Errorf("pool_slot %q is not a number", s)
		}
		n = parsed
	default:
		return 0, fmt.Errorf("pool_slot must be a whole number, not %v", v)
	}
	if !manifest.ValidPoolSlots(n) {
		return 0, fmt.Errorf("pool_slot %d is not a slot count: use a number above zero, or -1 for no limit", n)
	}
	return n, nil
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
			if !blankConn(sc) {
				c.blockers = append(c.blockers, SettingsRelPath+": connection "+strconv.Itoa(i+1)+
					connClues(sc)+" has no conn_id. Add one, or delete the entry")
			}
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

		// The value first, because a conn_uri is where the conn_type comes from
		// when the entry does not spell one out — and the declaration has to say
		// what the connection actually is.
		value, connType, err := connValue(sc)
		if err != nil {
			c.blockers = append(c.blockers, SettingsRelPath+": "+id+" cannot be carried. "+err.Error())
			continue
		}

		// A connection with no conn_type at all is refused rather than carried.
		//
		// Airflow chooses the provider from it, so a record without one is not
		// a connection it can resolve: carrying it stores something unusable and
		// declares it required, which stops the project starting over a value
		// that would not have worked.
		//
		// It is also where the two codecs disagree — this package's encoder is
		// happy with an empty conn_type and the app's decoder is not, so one
		// that gets through converts from the CLI and fails in the app. Refusing
		// it where the connection is authored settles that for both.
		if connType == "" {
			c.blockers = append(c.blockers, SettingsRelPath+": "+id+
				" has no conn_type, so Airflow cannot tell what kind of connection it is")
			continue
		}

		spec := envschema.ValueSpec{ConnType: connType, Sensitive: true}
		if problems := spec.Check(envschema.SectionConnection); len(problems) > 0 {
			for _, p := range problems {
				c.blockers = append(c.blockers, SettingsRelPath+": "+id+" cannot be carried. "+p.Reason)
			}
			continue
		}
		c.schema.Connections[id] = spec

		if value == "" {
			// Nothing to store. The declaration still goes in, so the project
			// says the connection is expected and refuses to start until it is
			// set — which is what an empty entry in the v1 file amounted to
			// anyway, minus the part where nothing said so.
			c.advisories = append(c.advisories, id+
				": declared as a required connection, with no value to carry. "+
				"Set it with `astro local env connection set "+id+" --secret`")
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
	for i, v := range vars {
		name := strings.TrimSpace(v.Name)
		if name == "" {
			if v.Value != "" {
				c.blockers = append(c.blockers, SettingsRelPath+": variable "+strconv.Itoa(i+1)+
					" has a variable_value but no variable_name. Add one, or delete the entry")
			}
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
		if _, err := secrets.Key(secrets.KindVar, secrets.GlobalScope, name); err != nil {
			c.blockers = append(c.blockers, SettingsRelPath+": "+name+" cannot be a vault key. "+err.Error())
			continue
		}
		if v.Value == "" {
			// Nothing to store, and nothing to require either. v1 skipped an
			// empty Variable rather than create it, so a Dag that reads it with
			// a fallback ran as if it were unset. Declaring it required would
			// turn that working project into one that refuses to start, so it
			// is declared optional: the name is recorded, and start resolves it
			// as absent until someone sets it.
			c.schema.AirflowVariables[name] = envschema.ValueSpec{Sensitive: true, HasSensitive: true, Optional: true}
			c.advisories = append(c.advisories, name+
				": declared as an optional Airflow variable, since it had no value to carry. "+
				"Set it with `astro local env airflow-variable set "+name+" --secret`")
			continue
		}
		c.schema.AirflowVariables[name] = envschema.ValueSpec{Sensitive: true, HasSensitive: true}
		c.secrets = append(c.secrets, SecretWrite{
			Kind:  secrets.KindVar,
			Name:  name,
			Label: "Airflow variable " + name,
			value: v.Value,
		})
	}
}

// blankConn reports that an entry with no conn_id has nothing else filled in
// either. v1 skipped such an entry, and the airflow_settings.yaml that v1's
// `astro dev init` wrote holds one, so it is skipped here too. That template's
// placeholder conn_extra counts as blank; any other extra does not.
func blankConn(sc *settingsConn) bool {
	fields := []string{sc.ConnType, sc.ConnHost, sc.ConnSchema, sc.ConnLogin, sc.ConnPassword, sc.ConnURI}
	if slices.ContainsFunc(fields, func(v string) bool { return strings.TrimSpace(v) != "" }) {
		return false
	}
	if port, err := connPort(sc.ConnPort); err != nil || port != 0 {
		return false
	}
	extra, err := connExtra(sc.ConnExtra)
	if err != nil {
		return false
	}
	return len(extra) == 0 || maps.Equal(extra, map[string]any{"example_extra_field": "example-value"})
}

// blankPool reports that an entry with no pool_name has nothing else filled
// in either, as in the template blankConn describes.
func blankPool(p *settingsPool) bool {
	return (p.Slot == nil || p.Slot == "") && strings.TrimSpace(p.Description) == ""
}

// connClues names what an entry with no conn_id does have, so the report can
// point at it: " (conn_type postgres, conn_host db.example.com)", or "" when
// neither is set.
func connClues(sc *settingsConn) string {
	var clues []string
	if t := strings.TrimSpace(sc.ConnType); t != "" {
		clues = append(clues, "conn_type "+t)
	}
	if h := strings.TrimSpace(sc.ConnHost); h != "" {
		clues = append(clues, "conn_host "+h)
	}
	if len(clues) == 0 {
		return ""
	}
	return " (" + strings.Join(clues, ", ") + ")"
}

// connValue is the vault payload for one connection: the canonical JSON, or
// the empty string when the entry supplied nothing to store.
//
// Both arms end at airflowenv, which is the one definition of what a stored
// connection looks like. A conn_uri is normalized through the same function
// `astro local env connection set --secret` uses, rather than stored as written: the vault
// holds JSON, and a URI sitting in it is a record only one of the two tools can
// read back.
//
// conn_uri wins over the broken-out fields when both are set, because that is
// how the v1 file spells a connection whose parts it did not enumerate — and
// merging the two would invent a connection the user never wrote.
//
// It also reports the conn_type it resolved, which for a URI is its scheme and
// is otherwise the field. The caller declares that rather than the raw field,
// so a connection written as a URI is declared as the kind it actually is.
func connValue(sc *settingsConn) (value, connType string, err error) {
	if uri := strings.TrimSpace(sc.ConnURI); uri != "" {
		id := strings.TrimSpace(sc.ConnID)
		parsed, perr := airflowenv.ConnFromURI(id, uri)
		if perr != nil {
			return "", "", perr
		}
		normalized, nerr := airflowenv.NormalizeConn(id, uri)
		if nerr != nil {
			return "", "", nerr
		}
		return normalized, parsed.ConnType, nil
	}
	connType = strings.TrimSpace(sc.ConnType)
	port, perr := connPort(sc.ConnPort)
	if perr != nil {
		return "", "", perr
	}
	extra, xerr := connExtra(sc.ConnExtra)
	if xerr != nil {
		return "", "", xerr
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
		return "", connType, nil
	}
	_, encoded, ok := airflowenv.EncodeConnEnv(c)
	if !ok {
		return "", "", fmt.Errorf("connection %q: could not encode value", c.ConnID)
	}
	return encoded, connType, nil
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

// retirable reports that the conversion leaves nothing behind in the file: it
// was carried, every value in it reaches the vault, and every pool reaches the
// manifest.
func (c *carriedSettings) retirable() bool {
	return c.schema != nil && len(c.pools.notes) == 0 && len(c.held) == 0 && !c.unstored
}

// setPools writes the carried pools into [tool.astro.pools].
func setPools(ed tomledit.Editor, pools map[string]manifest.Pool) error {
	for _, name := range slices.Sorted(maps.Keys(pools)) {
		table := map[string]any{"slots": pools[name].Slots}
		if d := pools[name].Description; d != "" {
			table["description"] = d
		}
		if err := ed.Set([]string{"tool", "astro", "pools", name}, table); err != nil {
			return err
		}
	}
	return nil
}

// poolsLabel is the manifest line saying the pools moved, or "" when there
// were none.
func poolsLabel(pools map[string]manifest.Pool) string {
	if len(pools) == 0 {
		return ""
	}
	return manifest.Marker + " (migrated " + plural(len(pools), "pool", "pools") + " from " + SettingsRelPath + " into [tool.astro.pools])"
}

// keptFor says why a carried file stays, for the advisory that says its
// plaintext is still on disk. Empty when the file is retired.
func (c *carriedSettings) keptFor() string {
	switch {
	case c.retirable():
		return ""
	case len(c.pools.notes) > 0:
		return "for a pool entry that could not be carried"
	case len(c.held) > 0:
		return "because " + c.heldPhrase() + ", so the file's value was not carried over it"
	}
	return "because the vault could not be read"
}

// checkVault asks which carried values the vault already holds. Apply keeps
// those rather than write the file's over them, which leaves the file as the
// only copy of its own, so Plan has to know before it retires the file. A
// vault that cannot be asked keeps the file too, and Apply then fails on the
// same question.
func (c *carriedSettings) checkVault(w SecretWriter) {
	for i := range c.secrets {
		h, err := checkHeld(w, &c.secrets[i])
		if err != nil {
			c.unstored = true
			return
		}
		if h.conflicts() {
			c.held = append(c.held, c.secrets[i].Name)
			if c.heldIn == nil {
				c.heldIn = map[string]SecretHeld{}
			}
			c.heldIn[c.secrets[i].Name] = h
		}
	}
}

// heldPhrase says where the held names' other values are: "the vault already
// held a and b", or one clause per place, "the vault already held a, and .env
// already held b".
func (c *carriedSettings) heldPhrase() string {
	byPlace := map[string][]string{}
	for _, name := range c.held {
		p := c.heldIn[name].place()
		byPlace[p] = append(byPlace[p], name)
	}
	var clauses []string
	for _, p := range slices.Sorted(maps.Keys(byPlace)) {
		clauses = append(clauses, p+" already held "+joinNames(byPlace[p]))
	}
	return strings.Join(clauses, ", and ")
}

// valueCount counts the values a run stores, by kind, as prose: "1 connection",
// "2 connections and 1 Airflow variable".
func valueCount(writes []SecretWrite) string {
	var conns, vars int
	for _, w := range writes {
		if w.Kind == secrets.KindConn {
			conns++
		} else {
			vars++
		}
	}
	var parts []string
	if conns > 0 {
		parts = append(parts, plural(conns, "connection", "connections"))
	}
	if vars > 0 {
		parts = append(parts, plural(vars, "Airflow variable", "Airflow variables"))
	}
	return strings.Join(parts, " and ")
}

// setCommands names the command that stores each kind of value writes holds.
func setCommands(writes []SecretWrite) string {
	var cmds []string
	if slices.ContainsFunc(writes, func(w SecretWrite) bool { return w.Kind == secrets.KindConn }) {
		cmds = append(cmds, "`astro local env connection set <id> --secret`")
	}
	if slices.ContainsFunc(writes, func(w SecretWrite) bool { return w.Kind == secrets.KindVar }) {
		cmds = append(cmds, "`astro local env airflow-variable set <key> --secret`")
	}
	return strings.Join(cmds, " and ")
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
