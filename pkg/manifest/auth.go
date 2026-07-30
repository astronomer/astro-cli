package manifest

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// AuthMethod is how the CLI proves itself to a link's Airflow. It is its own
// axis: any method attaches to any kind of link, and the kind only picks the
// default, because where an Airflow is and how it checks callers are separate
// questions — a self-hosted Airflow behind Google IAP is a url link with the
// google method, an Airflow 3 under Keycloak a url link with airflow-token.
type AuthMethod string

const (
	// AuthAstro takes the current session's bearer, or ASTRO_API_TOKEN.
	AuthAstro AuthMethod = "astro"
	// AuthAWS takes the AWS credential chain, for MWAA's own API doors.
	AuthAWS AuthMethod = "aws"
	// AuthGoogle takes Application Default Credentials.
	AuthGoogle AuthMethod = "google"
	// AuthBasic takes a username and password from named env vars.
	AuthBasic AuthMethod = "basic"
	// AuthToken takes a static bearer from a named env var.
	AuthToken AuthMethod = "token"
	// AuthAirflowToken exchanges credentials from named env vars at the
	// instance's own /auth/token.
	AuthAirflowToken AuthMethod = "airflow-token"
	// AuthExec runs a command and reads a token from its output — the
	// kubectl exec-plugin escape hatch, for anything the menu does not cover.
	AuthExec AuthMethod = "exec"
	// AuthNone sends no credential: local instances, open dev servers.
	AuthNone AuthMethod = "none"
)

// authMethods is the closed menu, in the order the messages list it.
var authMethods = []AuthMethod{AuthAstro, AuthAWS, AuthGoogle, AuthBasic, AuthToken, AuthAirflowToken, AuthExec, AuthNone}

// Auth is a link's resolved auth table. Method is always set on a manifest
// that validates: the link's own `method` when it declares an auth table, else
// its kind's default. The remaining fields are the ones that method takes, and
// every credential arrives as the *name* of an env var — no literal secret is
// legal in a committed file, so a field holding one does not exist here.
type Auth struct {
	Method AuthMethod
	// TokenEnv names the env var holding a bearer token (token).
	TokenEnv string
	// UsernameEnv and PasswordEnv name the env vars holding a username and
	// password (basic, airflow-token).
	UsernameEnv string
	PasswordEnv string
	// ClientIDEnv and ClientSecretEnv name the env vars holding OAuth client
	// credentials (airflow-token).
	ClientIDEnv     string
	ClientSecretEnv string
	// Command is exec's argv: the program to run and its arguments, run
	// directly and never through a shell.
	Command []string
}

// commandField is exec's one field, named here because it decodes as an array
// while every other field is a string.
const commandField = "command"

// authSpec is what one method takes: the fields it accepts, in the order
// messages list them, the ones it cannot work without, and any credentials
// that come in pairs.
type authSpec struct {
	fields   []string
	required []string
	// pairs are credentials where each half needs the other, and exactly one
	// whole pair belongs in the table.
	pairs [][2]string
}

// authSpecs describes every method that takes fields. A method missing from
// the map takes none: it derives its credential from a session or an SDK's own
// chain, so there is nothing for the manifest to name.
var authSpecs = map[AuthMethod]authSpec{
	AuthBasic: {
		fields:   []string{"username-env", "password-env"},
		required: []string{"username-env", "password-env"},
	},
	AuthToken: {fields: []string{"token-env"}, required: []string{"token-env"}},
	AuthExec:  {fields: []string{commandField}, required: []string{commandField}},
	// airflow-token's credentials come in pairs because Airflow 3's token
	// endpoint accepts different ones per auth manager: client id and secret
	// under Keycloak, username and password under FAB or the simple auth
	// manager. One pair is the credential; two would leave the resolver
	// picking, so the table names the pair its instance wants.
	AuthAirflowToken: {
		fields: []string{"client-id-env", "client-secret-env", "username-env", "password-env"},
		pairs:  [][2]string{{"client-id-env", "client-secret-env"}, {"username-env", "password-env"}},
	},
}

// envNameRe is what a *-env field must name: an env var, not a value.
var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// defaultAuthMethod is the method a link falls back to when it declares no
// auth table. An endpoint link is missing on purpose: nothing about a bare URL
// says how its Airflow checks callers, so it has no default and gets an empty
// method, which auth turns into a problem.
var defaultAuthMethod = map[LinkKind]AuthMethod{
	KindAstro:    AuthAstro,
	KindMWAA:     AuthAWS,
	KindComposer: AuthGoogle,
}

// auth types a link's `auth` table, which arrives untyped because its shape
// depends on the method it names. key is the table's dotted TOML key and kind
// the link's, for the default. Decoding is strict, like the env schema's: an
// unknown method, a field the method does not take, a missing credential
// field, and a misshapen table are all problems — this is authored config, and
// a typo that silently dropped a credential would surface much later as an
// unexplained 401.
func (p *parser) auth(key string, raw any, kind LinkKind) Auth {
	switch table := raw.(type) {
	case nil:
		method := defaultAuthMethod[kind]
		if method == "" {
			p.add(key, "required on a url link: nothing about a url says how its Airflow checks callers — add an auth table naming one of: "+methodList())
			return Auth{}
		}
		return Auth{Method: method}
	case map[string]any:
		return p.authTable(key, table)
	default:
		p.add(key, "must be a table like { method = 'token', token-env = 'AIRFLOW_TOKEN' }")
		return Auth{}
	}
}

func (p *parser) authTable(key string, table map[string]any) Auth {
	ap := &authParser{parser: p, key: key, seen: map[string]bool{}}
	method, ok := ap.method(table["method"])
	if !ok {
		return Auth{}
	}
	a := Auth{Method: method}
	spec := authSpecs[method]
	// Ranged in map order: every problem here is keyed by its own field, and
	// Parse sorts the whole set by key before reporting it.
	for field, raw := range table {
		if field == "method" {
			continue
		}
		if !slices.Contains(spec.fields, field) {
			ap.addField(field, notAField(method, spec.fields))
			continue
		}
		ap.seen[field] = true
		if field == commandField {
			a.Command = ap.command(raw)
			continue
		}
		*authDest(&a, field) = ap.value(field, raw)
	}
	ap.required(method, spec)
	return a
}

// authDest points at the Auth field a manifest field name fills. Names are
// global: a field means the same thing under every method that takes it. Only
// the string fields are here — exec's command is an array and decodes on its
// own path.
func authDest(a *Auth, field string) *string {
	switch field {
	case "token-env":
		return &a.TokenEnv
	case "username-env":
		return &a.UsernameEnv
	case "password-env":
		return &a.PasswordEnv
	case "client-id-env":
		return &a.ClientIDEnv
	case "client-secret-env":
		return &a.ClientSecretEnv
	default:
		// Unreachable: the caller matched the field against the method's spec,
		// and every string field in a spec is listed above.
		return new(string)
	}
}

// authParser decodes one auth table: the shared parser, the table's key, and
// which fields the table set. seen records a field whatever its value, so the
// required checks ask what was written rather than what decoded, and a field
// with a bad value is reported once instead of also being reported missing.
type authParser struct {
	*parser
	key  string
	seen map[string]bool
}

// addField reports a problem against one field of this table.
func (p *authParser) addField(field, reason string) {
	p.add(p.key+"."+field, reason)
}

// method decodes the `method` field, the one field every auth table must
// carry: the rest of the table means nothing until the method is known.
func (p *authParser) method(raw any) (AuthMethod, bool) {
	if raw == nil {
		p.addField("method", "required — name one of: "+methodList())
		return "", false
	}
	s, ok := raw.(string)
	if !ok {
		p.addField("method", "expected a string")
		return "", false
	}
	m := AuthMethod(s)
	if !slices.Contains(authMethods, m) {
		p.addField("method", fmt.Sprintf("%q is not an auth method — name one of: %s", s, methodList()))
		return "", false
	}
	return m, true
}

// value decodes one string field. A *-env field must name an env var rather
// than hold a value, which is also the check that keeps literal secrets out of
// the file.
func (p *authParser) value(field string, raw any) string {
	s, ok := raw.(string)
	if !ok {
		p.addField(field, "expected a string")
		return ""
	}
	if s == "" {
		p.addField(field, "must not be empty")
		return ""
	}
	if strings.HasSuffix(field, "-env") && !envNameRe.MatchString(s) {
		p.addField(field, fmt.Sprintf("%q is not an env-var name (letters, digits, _; no leading digit) — this field names the variable holding the value, not the value", s))
		return ""
	}
	return s
}

// command decodes exec's argv. It is an array rather than a string because the
// CLI runs the program directly and never through a shell: a string would
// promise quoting and word splitting that nothing implements.
func (p *authParser) command(raw any) []string {
	items, ok := raw.([]any)
	if !ok {
		p.addField(commandField, "expected an array, argv style: command = ['acme-airflow-token', '--profile', 'prod']")
		return nil
	}
	if len(items) == 0 {
		p.addField(commandField, "must name a program to run")
		return nil
	}
	out := make([]string, 0, len(items))
	for i, item := range items {
		s, ok := item.(string)
		if !ok || s == "" {
			p.add(fmt.Sprintf("%s.%s[%d]", p.key, commandField, i), "expected a non-empty string")
			continue
		}
		out = append(out, s)
	}
	return out
}

// required checks the fields the method cannot work without. A method with no
// spec names nothing: astro reads the session, aws and google their SDK's
// credential chain, none sends nothing.
func (p *authParser) required(method AuthMethod, spec authSpec) {
	for _, field := range spec.required {
		if !p.seen[field] {
			p.addField(field, fmt.Sprintf("required by the %s method", method))
		}
	}
	if len(spec.pairs) == 0 {
		return
	}
	whole := 0
	for _, pair := range spec.pairs {
		switch {
		case p.seen[pair[0]] && p.seen[pair[1]]:
			whole++
		case p.seen[pair[0]]:
			p.addField(pair[1], "required alongside "+pair[0])
		case p.seen[pair[1]]:
			p.addField(pair[0], "required alongside "+pair[1])
		}
	}
	switch {
	// seen holds only this method's own fields, so an empty set means the
	// table named no credential at all; half a pair has reported itself.
	case len(p.seen) == 0:
		p.add(p.key, fmt.Sprintf("the %s method needs credentials to exchange: %s", method, pairList(spec.pairs)))
	case whole > 1:
		p.add(p.key, fmt.Sprintf("the %s method takes one credential pair, not both: %s", method, pairList(spec.pairs)))
	}
}

func notAField(method AuthMethod, allowed []string) string {
	if len(allowed) == 0 {
		return fmt.Sprintf("not a field of the %s method, which takes none", method)
	}
	return fmt.Sprintf("not a field of the %s method (%s)", method, strings.Join(allowed, ", "))
}

// pairList spells the credential pairs a method accepts, as
// "a with b, or c with d".
func pairList(pairs [][2]string) string {
	each := make([]string, len(pairs))
	for i, pair := range pairs {
		each[i] = pair[0] + " with " + pair[1]
	}
	return strings.Join(each, ", or ")
}

func methodList() string {
	names := make([]string, len(authMethods))
	for i, m := range authMethods {
		names[i] = string(m)
	}
	return strings.Join(names, ", ")
}
