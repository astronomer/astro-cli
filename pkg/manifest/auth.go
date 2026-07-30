package manifest

import (
	"fmt"
	"maps"
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
	// Command is the command exec runs to print a token.
	Command string
}

// authFields lists the fields each method takes. A method missing from the map
// takes none: it derives its credential from a session or an SDK's own chain,
// so there is nothing for the manifest to name.
var authFields = map[AuthMethod][]string{
	AuthBasic:        {"username-env", "password-env"},
	AuthToken:        {"token-env"},
	AuthAirflowToken: {"client-id-env", "client-secret-env", "username-env", "password-env"},
	AuthExec:         {"command"},
}

// envNameRe is what a *-env field must name: an env var, not a value.
var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// defaultAuthMethod is the method a link falls back to when it declares no
// auth table. An endpoint link has no default — nothing about a bare URL says
// how its Airflow checks callers — and gets an empty method, which parseAuth
// turns into a problem.
func defaultAuthMethod(k Kind) AuthMethod {
	switch k {
	case KindAstro:
		return AuthAstro
	case KindMWAA:
		return AuthAWS
	case KindComposer:
		return AuthGoogle
	case KindEndpoint:
		return ""
	}
	return ""
}

// parseAuth types a link's `auth` table, which arrives untyped because its
// shape depends on the method it names. key is the table's dotted TOML key and
// kind the link's, for the default. Decoding is strict, like the env schema's:
// an unknown method, a field the method does not take, a missing credential
// field, and a misshapen table are all problems — this is authored config, and
// a typo that silently dropped a credential would surface much later as an
// unexplained 401.
func parseAuth(key string, raw any, kind Kind) (Auth, []Problem) {
	switch table := raw.(type) {
	case nil:
		method := defaultAuthMethod(kind)
		if method == "" {
			return Auth{}, []Problem{{
				Key:    key,
				Reason: "required on a url link: nothing about a url says how its Airflow checks callers — add auth = { method = '…' }, naming one of " + methodList(),
			}}
		}
		return Auth{Method: method}, nil
	case map[string]any:
		return parseAuthTable(key, table)
	default:
		return Auth{}, []Problem{{Key: key, Reason: "must be a table like { method = 'token', token-env = 'AIRFLOW_TOKEN' }"}}
	}
}

func parseAuthTable(key string, table map[string]any) (Auth, []Problem) {
	p := &authParser{key: key, seen: map[string]bool{}}
	method, ok := p.method(table["method"])
	if !ok {
		return Auth{}, p.problems
	}
	a := Auth{Method: method}
	allowed := authFields[method]
	// Sorted, because a map's order must not decide which problem is reported
	// first — the caller sorts by key, and two fields can share none.
	for _, field := range slices.Sorted(maps.Keys(table)) {
		if field == "method" {
			continue
		}
		dest := authDest(&a, field)
		if dest == nil || !slices.Contains(allowed, field) {
			p.add(key+"."+field, notAField(method, allowed))
			continue
		}
		p.seen[field] = true
		*dest = p.value(key+"."+field, field, table[field])
	}
	p.required(method)
	return a, p.problems
}

// authDest points at the Auth field a manifest field name fills. Names are
// global: a field means the same thing under every method that takes it.
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
	case "command":
		return &a.Command
	default:
		return nil
	}
}

// authParser collects one auth table's problems. seen records the fields the
// table set, whatever their value: the required checks ask what was written,
// not what decoded, so a field with a bad value is reported once rather than
// also being reported missing.
type authParser struct {
	key      string
	seen     map[string]bool
	problems []Problem
}

func (p *authParser) add(key, reason string) {
	p.problems = append(p.problems, Problem{Key: key, Reason: reason})
}

// method decodes the `method` field, the one field every auth table must
// carry: the rest of the table means nothing until the method is known.
func (p *authParser) method(raw any) (AuthMethod, bool) {
	if raw == nil {
		p.add(p.key+".method", "required: "+methodList())
		return "", false
	}
	s, ok := raw.(string)
	if !ok {
		p.add(p.key+".method", "expected a string")
		return "", false
	}
	m := AuthMethod(s)
	if !slices.Contains(authMethods, m) {
		p.add(p.key+".method", fmt.Sprintf("%q is not an auth method (%s)", s, methodList()))
		return "", false
	}
	return m, true
}

// value decodes one field. A *-env field must name an env var rather than hold
// a value, which is also the check that keeps literal secrets out of the file.
func (p *authParser) value(key, field string, raw any) string {
	s, ok := raw.(string)
	if !ok {
		p.add(key, "expected a string")
		return ""
	}
	if s == "" {
		p.add(key, "must not be empty")
		return ""
	}
	if strings.HasSuffix(field, "-env") && !envNameRe.MatchString(s) {
		p.add(key, fmt.Sprintf("%q is not an env-var name (letters, digits, _; no leading digit) — this field names the variable holding the value, not the value", s))
		return ""
	}
	return s
}

// required checks the fields the method cannot work without. The methods with
// no entry here name nothing: astro reads the session, aws and google their
// SDK's credential chain, none sends nothing.
func (p *authParser) required(method AuthMethod) {
	switch method {
	case AuthToken:
		p.must("token-env")
	case AuthBasic:
		p.must("username-env")
		p.must("password-env")
	case AuthExec:
		p.must("command")
	case AuthAirflowToken:
		p.airflowTokenPairs()
	case AuthAstro, AuthAWS, AuthGoogle, AuthNone:
	}
}

func (p *authParser) must(field string) {
	if !p.seen[field] {
		p.add(p.key+"."+field, "required by this method")
	}
}

// airflowTokenPairs checks the airflow-token credentials, which come in pairs
// because Airflow 3's token endpoint accepts different ones per auth manager:
// client id and secret under Keycloak, username and password under FAB or the
// simple auth manager. Either pair on its own is enough, both together are
// legal (a client-credentials grant that also carries user credentials), and
// half a pair is always a mistake.
func (p *authParser) airflowTokenPairs() {
	p.pair("client-id-env", "client-secret-env")
	p.pair("username-env", "password-env")
	// seen holds only the method's own fields, so an empty set means the table
	// named no credential at all.
	if len(p.seen) == 0 {
		p.add(p.key, "the airflow-token method needs credentials to exchange: client-id-env with client-secret-env, or username-env with password-env")
	}
}

// pair reports half a credential pair, which is never what anyone meant.
func (p *authParser) pair(firstField, secondField string) {
	switch {
	case !p.seen[firstField] && p.seen[secondField]:
		p.add(p.key+"."+firstField, "required alongside "+secondField)
	case !p.seen[secondField] && p.seen[firstField]:
		p.add(p.key+"."+secondField, "required alongside "+firstField)
	}
}

func notAField(method AuthMethod, allowed []string) string {
	if len(allowed) == 0 {
		return fmt.Sprintf("not a field of the %s method, which takes none", method)
	}
	return fmt.Sprintf("not a field of the %s method (%s)", method, strings.Join(allowed, ", "))
}

func methodList() string {
	names := make([]string, len(authMethods))
	for i, m := range authMethods {
		names[i] = string(m)
	}
	return strings.Join(names, ", ")
}
