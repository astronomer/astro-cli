package scaffold

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// The deployment link writer: [tool.astro.deployments], the shared
// [tool.astro.targets.<kind>] coordinates those links read, and the top-level
// workspace link. Every edit, from the CLI and from Astro Desktop, goes through
// SaveLink, RemoveLink, SetDefaultLink or SetWorkspaceLink, and each of those
// goes through EditManifest, so the write rules there hold for all of them.
//
// A link follows what it inherits. [tool.astro] may set a workspace and a
// target every link without its own falls back to, and a link that repeated
// the project's value would stop following it: a later change to the
// top-level key would reach every link except the ones this writer saved. So
// SaveLink writes a link's target and workspace only where they differ from
// what it would inherit, and keeps a key the link already sets itself, since
// whoever wrote it pinned the link on purpose. SetWorkspaceLink is the other
// half: before the top-level workspace changes, each astro link inheriting it
// has the old value written onto itself, so no link moves to a workspace its
// deployment is not in.

// ErrInvalidLink reports a link SaveLink refused before reading the manifest,
// because the parser would refuse it. The message names the field.
var ErrInvalidLink = errors.New("invalid link")

// ErrComposerProjectRequired and ErrComposerLocationRequired report a Composer
// link saved without the Google Cloud project or the region holding its
// environment. The parser does not require them, but an environment name alone
// does not say where the environment is, so a link without both could never be
// resolved to an address. Both wrap ErrInvalidLink.
var (
	ErrComposerProjectRequired  = fmt.Errorf("%w: a Cloud Composer link needs the Google Cloud project holding the environment", ErrInvalidLink)
	ErrComposerLocationRequired = fmt.Errorf("%w: a Cloud Composer link needs the region holding the environment", ErrInvalidLink)
)

// ErrNoSuchLink reports SetDefaultLink naming a link the manifest does not
// have. Nothing is written.
var ErrNoSuchLink = errors.New("no such link")

// ErrWorkspaceDomainRequired reports SetWorkspaceLink linking a workspace with
// no domain. The domain picks the login the workspace is read with
// (docs/v2-workspace-link.md), so a link without one cannot be written.
var ErrWorkspaceDomainRequired = errors.New("linking a workspace needs the Astro domain it lives on")

// Link is one deployment link as a caller describes it: the name it is keyed
// by, the coordinates of what it points at, and how it authenticates.
//
// Kind is not written to the manifest. The parser derives it from which
// coordinates a link carries, so it tells SaveLink which of them to write.
type Link struct {
	Name string
	Kind manifest.LinkKind

	// Workspace and Deployment are an astro link's coordinates. An empty
	// Workspace keeps the workspace the link already sets itself, which is
	// how SetWorkspaceLink pins a link, and otherwise inherits [tool.astro]
	// workspace.
	Workspace  string
	Deployment string
	// ClearWorkspace removes the workspace the link sets itself when Workspace
	// is empty, so the link inherits [tool.astro] workspace again. It is a
	// separate request for the same reason ClearTargetRegion is: an empty
	// Workspace also means "not given".
	ClearWorkspace bool
	// Environment is the platform's own name on an mwaa or composer link.
	Environment string
	// URL is an endpoint link's Airflow base URL.
	URL string

	// Auth names the method and the env vars its credentials come from, never
	// a credential. An empty Method takes the kind's default, which an
	// endpoint link does not have.
	Auth manifest.Auth

	// TargetProject and TargetLocation are the Google Cloud project and region
	// holding a Composer environment, written to [tool.astro.targets.composer].
	// Required on a Composer link.
	//
	// They are project-level, not per-link: every Composer link in a project
	// reads the same section, so saving one link's coordinates changes what the
	// others resolve to.
	TargetProject  string
	TargetLocation string

	// TargetRegion is the AWS region holding an MWAA environment, written to
	// [tool.astro.targets.mwaa] and shared by every MWAA link the same way.
	// Optional: without one the AWS SDK's own region chain answers, so an empty
	// TargetRegion leaves the section as it is.
	TargetRegion string
	// ClearTargetRegion removes that shared region when TargetRegion is empty.
	// It is a separate request because an empty TargetRegion also means "not
	// given" to a caller that never set it.
	ClearTargetRegion bool
}

// The [tool.astro.targets.<kind>] keys the writer sets. The sections are plain
// data to the parser; these are the ones the address lookups read
// (pkg/instancelocate for Composer, pkg/awsauth for MWAA), and no other key in
// them is touched.
const (
	targetProjectField  = "project"
	targetLocationField = "location"
	targetRegionField   = "region"
)

// linkAuthFields is what each auth method takes, mirroring pkg/manifest's own
// authSpecs. A method absent from it takes none. The parser refuses a table
// carrying a field its method does not take, so a save writes only these,
// whatever else the caller's Auth still holds from an earlier method.
var linkAuthFields = map[manifest.AuthMethod][]string{
	manifest.AuthBasic:        {"username-env", "password-env"},
	manifest.AuthToken:        {"token-env"},
	manifest.AuthExec:         {"command"},
	manifest.AuthAirflowToken: {"client-id-env", "client-secret-env", "username-env", "password-env"},
}

// linkDefaultAuth is the method a kind takes with no auth table, mirroring the
// parser's. An endpoint is absent: nothing about a url says how its Airflow
// checks callers. It is used only to leave out a table that would repeat the
// kind, so a mistake here produces a manifest the write-side parse refuses,
// never a link that authenticates differently than asked.
var linkDefaultAuth = map[manifest.LinkKind]manifest.AuthMethod{
	manifest.KindAstro:    manifest.AuthAstro,
	manifest.KindMWAA:     manifest.AuthAWS,
	manifest.KindComposer: manifest.AuthGoogle,
}

// linkEnvNameRe is the manifest's rule for an env var name, kept identical so a
// value the parser would refuse is refused here first, without the parser's
// message, which quotes the value.
var linkEnvNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// SaveLink writes l into the pyproject.toml in dir, creating the entry under
// its name or replacing the one there, through EditManifest. wrap is
// EditManifest's wrapper.
//
// The link's own table is replaced whole, except for what belongs to other
// operations or was pinned:
//
//   - target and workspace are written only where they differ from what the
//     link inherits from [tool.astro] (workspace, target, else astro). A key the
//     link already sets is kept, with the new value, even where that matches
//     the default. An endpoint's target is astro, since there is no endpoint
//     target to name, so it is written only in a project whose target is not.
//   - an auth table is written only when it says more than the kind's default
//     method, and carries only the fields its method takes.
//   - default = true is carried over when the link had it. The flag belongs to
//     SetDefaultLink, so a re-save does not hand the default to nobody.
//
// An empty Workspace keeps a workspace the link already sets itself;
// ClearWorkspace removes it. Removing the link and saving it again does too.
//
// A Composer link also sets the shared project and location, and an MWAA link
// the shared region when it gives one (or removes it on ClearTargetRegion),
// each leaf by leaf, so the section's header, comments and other keys stay.
//
// l is checked before the manifest is read, and a link the parser would refuse
// wraps ErrInvalidLink. A credential pasted where an env var name belongs is
// refused without being quoted back.
//
//nolint:gocritic // hugeParam: by value on purpose, so the trims below change SaveLink's copy and never the caller's
func SaveLink(dir string, wrap func(run func() error) error, l Link) error {
	l.Name = strings.TrimSpace(l.Name)
	// A coordinate of spaces passes an emptiness check and reaches the Composer
	// API as an escaped blank, which answers 404 and blames the environment.
	l.TargetProject = strings.TrimSpace(l.TargetProject)
	l.TargetLocation = strings.TrimSpace(l.TargetLocation)
	l.TargetRegion = strings.TrimSpace(l.TargetRegion)
	l.Workspace = strings.TrimSpace(l.Workspace)
	l.Deployment = strings.TrimSpace(l.Deployment)
	l.Environment = strings.TrimSpace(l.Environment)
	l.URL = strings.TrimSpace(l.URL)
	if err := l.validate(); err != nil {
		return err
	}
	return EditManifest(dir, wrap, l.edit())
}

// edit is SaveLink's change to the manifest, for a link already trimmed and
// validated. The v1 conversion applies it to the manifest it is building, so
// a converted project's link is written exactly as `astro link add` writes one.
//
//nolint:gocritic // hugeParam: by value, as SaveLink holds it
func (l Link) edit() ManifestEdit {
	return func(before *manifest.Manifest, ed tomledit.Editor) error {
		key := linkKey(l.Name)
		// Read from the raw TOML, before the entry is replaced: the parsed
		// link has the defaults folded in, so it cannot say which keys the link
		// wrote itself.
		pinned := func(field string) bool {
			_, ok := ed.Get(append(key, field))
			return ok
		}
		table := map[string]any{}
		switch l.Kind {
		case manifest.KindAstro:
			// Not given, the link keeps the workspace it sets itself: the
			// table is replaced whole below, and dropping a pin there would
			// move the link to [tool.astro] workspace. Never written empty:
			// the parser rejects an explicitly empty workspace.
			workspace := l.Workspace
			if workspace == "" && !l.ClearWorkspace {
				own, _ := ed.Get(append(key, manifestKeyWorkspace))
				workspace, _ = own.(string)
			}
			if workspace != "" && (workspace != before.Astro.Workspace || pinned("workspace")) {
				table["workspace"] = workspace
			}
			table["deployment"] = l.Deployment
		case manifest.KindMWAA, manifest.KindComposer:
			table["environment"] = l.Environment
		case manifest.KindEndpoint:
			table["url"] = l.URL
		}
		if target := l.target(); target != inheritedTarget(before) || pinned("target") {
			table["target"] = target
		}
		if auth := authTable(&l.Auth, l.Kind); auth != nil {
			table["auth"] = auth
		}
		if existing, ok := before.Astro.Deployments[l.Name]; ok && existing.Default {
			table["default"] = true
		}
		if err := setLinkTargets(ed, &l); err != nil {
			return err
		}
		return ReplaceTable(ed, key, table)
	}
}

// RemoveLink removes the link called name from the pyproject.toml in dir,
// through EditManifest, and reports whether there was one. Removing a link that
// is not there writes nothing and is not an error, so a caller does not have to
// check first.
//
// Removing the last link removes the [tool.astro.deployments] table it leaves
// empty, so the file does not keep a header with nothing under it. The shared
// [tool.astro.targets.<kind>] sections stay: other links may read them, and
// they are plain data when none does.
func RemoveLink(dir string, wrap func(run func() error) error, name string) (removed bool, err error) {
	name = strings.TrimSpace(name)
	err = EditManifest(dir, wrap, func(_ *manifest.Manifest, ed tomledit.Editor) error {
		removed = ed.Delete(linkKey(name))
		if !removed {
			return nil
		}
		if rest, ok := ed.Get(linksKey()); ok {
			if table, isTable := rest.(map[string]any); isTable && len(table) == 0 {
				ed.Delete(linksKey())
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return removed, nil
}

// SetDefaultLink marks the link called name as the default in the
// pyproject.toml in dir, and clears the flag from every other link, through
// EditManifest. An empty name clears it from all of them.
//
// Both halves, because the manifest allows at most one default: setting the
// flag without clearing the previous holder writes a file the parser refuses.
// A name the manifest has no link for reports ErrNoSuchLink.
func SetDefaultLink(dir string, wrap func(run func() error) error, name string) error {
	name = strings.TrimSpace(name)
	return EditManifest(dir, wrap, func(before *manifest.Manifest, ed tomledit.Editor) error {
		if name != "" {
			if _, ok := before.Astro.Deployments[name]; !ok {
				return fmt.Errorf("%w: %s", ErrNoSuchLink, name)
			}
		}
		for other := range before.Astro.Deployments {
			if other != name && before.Astro.Deployments[other].Default {
				ed.Delete(append(linkKey(other), "default"))
			}
		}
		if name == "" || before.Astro.Deployments[name].Default {
			return nil
		}
		return ed.Set(append(linkKey(name), "default"), true)
	})
}

// SetWorkspaceLink links the project in dir to the Astro workspace workspaceID
// on domain, or unlinks it when workspaceID is empty, through EditManifest.
//
// Linking writes [tool.astro] workspace and domain together, the domain
// normalized by manifest.NormalizeDomain the way `astro login` stores a login,
// so both apps look the login up under the same key. An empty domain is
// refused with ErrWorkspaceDomainRequired. Unlinking removes both keys, since
// a domain with no workspace is a manifest the parser refuses.
//
// The top-level workspace is also what an astro link with no workspace of its
// own resolves through. So whenever it changes, switched or removed, each such
// link first has the old value written onto itself and keeps pointing where it
// did: without that, a switch would read deployment clx1 in ws_A as clx1 in
// ws_B, and an unlink would leave a link the parser refuses. The domain does
// not reach a deployment link: those are looked up with the current login,
// never [tool.astro] domain (docs/v2-workspace-link.md), so changing or
// removing the domain does not move a pinned link either. Linking the
// workspace already linked copies nothing. pinned names the links the old
// value was written onto, sorted, and is empty when nothing was written.
func SetWorkspaceLink(dir string, wrap func(run func() error) error, workspaceID, domain string) (pinned []string, err error) {
	workspaceID = strings.TrimSpace(workspaceID)
	domain = manifest.NormalizeDomain(domain)
	if workspaceID != "" && domain == "" {
		return nil, ErrWorkspaceDomainRequired
	}
	err = EditManifest(dir, wrap, func(before *manifest.Manifest, ed tomledit.Editor) error {
		pinned = nil
		if before.Astro.Workspace != workspaceID {
			var perr error
			if pinned, perr = pinInheritingLinks(before, ed); perr != nil {
				return perr
			}
		}
		if workspaceID == "" {
			ed.Delete(astroKey(manifestKeyDomain))
			ed.Delete(astroKey(manifestKeyWorkspace))
			return nil
		}
		// Each key is set only when it changes, so linking what is already
		// linked leaves the file byte for byte as it was.
		if before.Astro.Workspace != workspaceID {
			if err := ed.Set(astroKey(manifestKeyWorkspace), workspaceID); err != nil {
				return err
			}
		}
		if before.Astro.Domain != domain {
			return ed.Set(astroKey(manifestKeyDomain), domain)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pinned, nil
}

// The [tool.astro] keys SetWorkspaceLink writes.
const (
	manifestKeyWorkspace = "workspace"
	manifestKeyDomain    = "domain"
)

func astroKey(field string) []string {
	return []string{"tool", "astro", field}
}

// linksKey is [tool.astro.deployments], the table every link lives in.
func linksKey() []string {
	return []string{"tool", "astro", "deployments"}
}

func linkKey(name string) []string {
	return []string{"tool", "astro", "deployments", name}
}

func linkTargetKey(kind manifest.LinkKind, field string) []string {
	return []string{"tool", "astro", "targets", string(kind), field}
}

// pinInheritingLinks writes the project's current workspace onto each astro
// link that sets none of its own, before that workspace is replaced or
// removed, and returns their names, sorted.
func pinInheritingLinks(before *manifest.Manifest, ed tomledit.Editor) ([]string, error) {
	if before.Astro.Workspace == "" {
		return nil, nil
	}
	var pinned []string
	for name := range before.Astro.Deployments {
		if before.Astro.Deployments[name].Kind() != manifest.KindAstro {
			continue
		}
		own := append(linkKey(name), manifestKeyWorkspace)
		if _, set := ed.Get(own); set {
			continue
		}
		if err := ed.Set(own, before.Astro.Workspace); err != nil {
			return nil, err
		}
		pinned = append(pinned, name)
	}
	sort.Strings(pinned)
	return pinned, nil
}

// setLinkTargets writes the shared target coordinates l's kind reads.
//
// One Set per field, never a Delete and Set of the section: replacing a table
// collapses it into an inline table under [tool.astro.targets], dropping its
// header, its comments and its key order. This section is the likeliest in the
// file to be commented, since it says which cloud account a team's
// environments live in. Setting a leaf also creates the section when it is
// absent.
func setLinkTargets(ed tomledit.Editor, l *Link) error {
	switch l.Kind {
	case manifest.KindComposer:
		if err := ed.Set(linkTargetKey(manifest.KindComposer, targetProjectField), l.TargetProject); err != nil {
			return err
		}
		return ed.Set(linkTargetKey(manifest.KindComposer, targetLocationField), l.TargetLocation)
	case manifest.KindMWAA:
		// An empty region alone is what a caller that never set the field
		// sends, and deleting the shared one then would move every other MWAA
		// link's lookup, so clearing takes the explicit flag.
		switch {
		case l.TargetRegion != "":
			return ed.Set(linkTargetKey(manifest.KindMWAA, targetRegionField), l.TargetRegion)
		case l.ClearTargetRegion:
			ed.Delete(linkTargetKey(manifest.KindMWAA, targetRegionField))
		}
	case manifest.KindAstro, manifest.KindEndpoint:
	}
	return nil
}

// target is the `target` a link of l's kind resolves through. An endpoint's is
// astro: a url is what makes a link an endpoint, and naming a platform beside
// one is a conflict the parser refuses.
func (l *Link) target() string {
	if l.Kind == manifest.KindEndpoint {
		return string(manifest.KindAstro)
	}
	return string(l.Kind)
}

// inheritedTarget is the target a link with none of its own resolves to:
// [tool.astro] target, else astro, the fallback the parser applies.
func inheritedTarget(m *manifest.Manifest) string {
	if m.Astro.Target != "" {
		return m.Astro.Target
	}
	return string(manifest.KindAstro)
}

// authTable is a as the manifest spells it, or nil when the kind's default
// method already says everything: the method matches and names no credential.
func authTable(a *manifest.Auth, kind manifest.LinkKind) map[string]any {
	if a.Method == "" || (a.Method == linkDefaultAuth[kind] && authNamesNothing(a)) {
		return nil
	}
	out := map[string]any{"method": string(a.Method)}
	for _, field := range linkAuthFields[a.Method] {
		if field == "command" {
			if len(a.Command) > 0 {
				out[field] = a.Command
			}
			continue
		}
		if value := authEnv(a, field); value != "" {
			out[field] = value
		}
	}
	return out
}

// authNamesNothing reports that a names no credential its method takes. A
// field only another method takes is left over from an earlier choice and is
// never written, so it does not count.
func authNamesNothing(a *manifest.Auth) bool {
	for _, field := range linkAuthFields[a.Method] {
		if field == "command" {
			if len(a.Command) > 0 {
				return false
			}
			continue
		}
		if authEnv(a, field) != "" {
			return false
		}
	}
	return true
}

// authEnv is a's value for one env-var-name field.
func authEnv(a *manifest.Auth, field string) string {
	switch field {
	case "token-env":
		return a.TokenEnv
	case "username-env":
		return a.UsernameEnv
	case "password-env":
		return a.PasswordEnv
	case "client-id-env":
		return a.ClientIDEnv
	case "client-secret-env":
		return a.ClientSecretEnv
	}
	return ""
}

// urlHasUserinfo reports a url whose authority carries a username or
// password, with or without a scheme in front: "admin:hunter2@host" parses as
// scheme "admin", so the check reads the text rather than url.Parse's User.
func urlHasUserinfo(raw string) bool {
	if _, rest, ok := strings.Cut(raw, "://"); ok {
		raw = rest
	}
	authority, _, _ := strings.Cut(raw, "/")
	authority, _, _ = strings.Cut(authority, "?")
	authority, _, _ = strings.Cut(authority, "#")
	return strings.Contains(authority, "@")
}

func invalidLink(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidLink, fmt.Sprintf(format, args...))
}

// validate refuses what the parser would refuse, with a message about the
// field rather than the file. It is not what makes the write safe, since
// EditManifest parses the result; it is what makes the refusal readable.
func (l *Link) validate() error {
	switch l.Name {
	case "":
		return invalidLink("a link needs a name")
	case manifest.ReservedLinkName:
		return invalidLink("%q is reserved: it always means the Airflow running on this machine", manifest.ReservedLinkName)
	}
	switch l.Kind {
	case manifest.KindAstro:
		if l.Deployment == "" {
			return invalidLink("an astro link needs a deployment")
		}
	case manifest.KindMWAA, manifest.KindComposer:
		if l.Environment == "" {
			return invalidLink("a %s link needs an environment", l.Kind)
		}
		if l.Kind == manifest.KindComposer {
			if l.TargetProject == "" {
				return ErrComposerProjectRequired
			}
			if l.TargetLocation == "" {
				return ErrComposerLocationRequired
			}
		}
	case manifest.KindEndpoint:
		if l.URL == "" {
			return invalidLink("an endpoint link needs a url")
		}
		// Refused here, and never quoted: the parser's own messages quote
		// the url, which would print the password.
		if urlHasUserinfo(l.URL) {
			return invalidLink("a url must not carry a username or password. Credentials are never saved: name the env vars that hold them with the basic auth method")
		}
		if l.Auth.Method == "" {
			return invalidLink("a url link needs an auth method: nothing about a url says how its Airflow checks callers")
		}
	default:
		return invalidLink("unknown link kind %q", l.Kind)
	}
	return validateAuth(&l.Auth)
}

// validateAuth checks that every credential field names an env var and that the
// method has the fields it cannot work without. A value that is not an env var
// name is almost always a credential pasted into the wrong field, so the
// message names the field and never the value.
//
// Only the fields the method takes are checked, since authTable writes no
// other: a field left over from an earlier method is dropped, not refused.
func validateAuth(a *manifest.Auth) error {
	for _, f := range []struct{ field, label, value string }{
		{"token-env", "token", a.TokenEnv},
		{"username-env", "username", a.UsernameEnv},
		{"password-env", "password", a.PasswordEnv},
		{"client-id-env", "client id", a.ClientIDEnv},
		{"client-secret-env", "client secret", a.ClientSecretEnv},
	} {
		if !slices.Contains(linkAuthFields[a.Method], f.field) {
			continue
		}
		if f.value != "" && !linkEnvNameRe.MatchString(f.value) {
			return invalidLink("the %s field takes the NAME of an environment variable, not a value, and what was given is not one", f.label)
		}
	}
	switch a.Method {
	case "", manifest.AuthAstro, manifest.AuthAWS, manifest.AuthGoogle, manifest.AuthNone:
		return nil
	case manifest.AuthToken:
		if a.TokenEnv == "" {
			return invalidLink("token auth needs the name of the environment variable holding the token")
		}
	case manifest.AuthBasic:
		if a.UsernameEnv == "" || a.PasswordEnv == "" {
			return invalidLink("basic auth needs both a username and a password variable")
		}
	case manifest.AuthExec:
		if len(a.Command) == 0 {
			return invalidLink("exec auth needs a command to run")
		}
	case manifest.AuthAirflowToken:
		return validateAuthPair(a)
	default:
		return invalidLink("unknown auth method %q", a.Method)
	}
	return nil
}

// validateAuthPair enforces airflow-token's rule: exactly one whole credential
// pair, a client id and secret or a username and password.
func validateAuthPair(a *manifest.Auth) error {
	pairs := 0
	for _, p := range [][2]string{{a.ClientIDEnv, a.ClientSecretEnv}, {a.UsernameEnv, a.PasswordEnv}} {
		if p[0] == "" && p[1] == "" {
			continue
		}
		if p[0] == "" || p[1] == "" {
			return invalidLink("airflow-token credentials come in pairs: name both halves, or neither")
		}
		pairs++
	}
	if pairs != 1 {
		return invalidLink("airflow-token takes exactly one credential pair: a client id and secret, or a username and password")
	}
	return nil
}
