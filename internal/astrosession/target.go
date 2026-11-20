package astrosession

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// This file is the one place that decides which Astro Workspace (and, for a
// Deployment-scoped token, which Deployment) a run acts on.
//
// Precedence, first that names one wins:
//
//  1. the explicit flag: --workspace (--deployment for the Deployment);
//  2. the project link: [tool.astro] in pyproject.toml, for the command
//     families that follow the project (Request.Project);
//  3. the token's claims: a Workspace or Deployment API token names its
//     Workspace, a Deployment token also its Deployment. An Organization
//     token names neither;
//  4. the saved context: the Workspace an interactive login last used. Never
//     under ASTRO_API_TOKEN, and never another host's.
//
// A run with ASTRO_API_TOKEN reads step 3 from the token itself (see
// TokenIdentity), in the platform tree and in the core tree alike, and never
// reads the context for it, nor for the host (see TokenHost): the saved
// login's Workspace and host are out of reach whether or not the root's setup
// overlaid the token on the context (it does not, in the core tree). The
// token's claims apply on the token run's host, whichever that is. Step 4 is
// only for a run with no token.
//
// A Target records where the answer came from, and, when there is none, what
// would supply one, so the "no Workspace" error and its advice are formed
// here, worded for what the command accepts.

// Source is where a run's Workspace or Deployment came from.
type Source uint8

const (
	// SourceNone is no source: nothing named one.
	SourceNone Source = iota
	// SourceFlag is --workspace or --deployment.
	SourceFlag
	// SourceProject is the project's [tool.astro] link.
	SourceProject
	// SourceToken is the claim of a Workspace or Deployment API token.
	SourceToken
	// SourceContext is the Workspace a saved interactive login last used.
	SourceContext
	// SourcePath is a Workspace id the person wrote in a request path.
	SourcePath
)

// The words a source goes by in a message.
const (
	flagWorkspace = "--workspace"
	labelPath     = "the request path"
)

// Label is the source as a parenthesis names it, "from --workspace" or "from
// the project link", for the Workspace (flag is "--workspace") or the
// Deployment ("--deployment") it chose. "" is no source: the saved login is
// the run's own and is not named where it is what anyone expected.
func (s Source) Label(flag string) string {
	switch s {
	case SourceFlag:
		return flag
	case SourceProject:
		return "the project link"
	case SourceToken:
		return EnvAPIToken
	case SourcePath:
		return labelPath
	case SourceNone, SourceContext:
	}
	return ""
}

// Annotate is the Workspace id with where the run took it from, "id (from
// --workspace)", for a Workspace a source the person can see chose (the flag,
// the project link, the token, a path), and the bare id for any other.
func Annotate(id string, s Source) string {
	if from := s.Label(flagWorkspace); from != "" {
		return fmt.Sprintf("%s (from %s)", id, from)
	}
	return id
}

// DescribeWorkspace is "workspace id (from --workspace)" for a Workspace a
// source the person can see chose, and "workspace id" for any other.
func DescribeWorkspace(id string, s Source) string {
	return "workspace " + Annotate(id, s)
}

// WorkspaceError is an error about looking in one Workspace (not found, no
// access, nothing in it). It carries the Workspace's id and where the run took
// it from, and its message names both: Before, then "workspace id (from
// --workspace)", then After. What built the error knew its Workspace and its
// source, so the message was formed with the source in it, and nothing rewrites
// it afterwards. It unwraps to Err, so the cause's kind and code are unchanged.
type WorkspaceError struct {
	// Workspace is the id of the Workspace looked in, Source where it came from.
	Workspace string
	Source    Source
	// Before and After are the message around the Workspace's description.
	Before, After string
	// Err is the cause, nil for none.
	Err error
}

func (e *WorkspaceError) Error() string {
	return e.Before + DescribeWorkspace(e.Workspace, e.Source) + e.After
}

func (e *WorkspaceError) Unwrap() error { return e.Err }

// From is the source worded for a message ("--workspace", "the project's
// link"), "" for none.
func (s Source) From() string {
	switch s {
	case SourceFlag:
		return flagWorkspace
	case SourceProject:
		return "the project's link"
	case SourceToken:
		return EnvAPIToken
	case SourceContext:
		return "the saved login"
	case SourcePath:
		return labelPath
	case SourceNone:
	}
	return ""
}

// WorkspaceAdvice is the ways a command lets a person name a Workspace,
// besides the token's claim and the saved login. It only picks the advice a
// missing Workspace gets (see Missing), so that never offers a way the command
// would not take. It does not decide which Workspace the run acts on.
type WorkspaceAdvice uint8

const (
	// ByFlag is --workspace. Every command has it.
	ByFlag WorkspaceAdvice = 1 << iota
	// ByProject is a project linked to a Workspace, for the command families
	// that follow the project.
	ByProject
	// ByID is the id of the token or Deployment the command acts on, which
	// identifies it with no Workspace to look in.
	ByID
	// ByPath is writing the Workspace's id where the command takes a path (the
	// astro api commands' placeholders).
	ByPath
	// ByDeployment is -d with the Deployment's ID, for a Deployment name that
	// has no Workspace to be found in.
	ByDeployment
)

// adviceFor is the sentence for each way, in the order they are offered.
var adviceFor = []struct {
	way  WorkspaceAdvice
	text string
}{
	{ByFlag, "pass --workspace"},
	{ByDeployment, "pass -d with the Deployment's id"},
	{ByProject, "run it in a project linked to a Workspace"},
	{ByID, "give the token's ID"},
	{ByPath, "write the Workspace's id in the path"},
}

// hintCurrent is for a command that acts on the current Workspace alone,
// which a run with an API token does not have.
const hintCurrent = "so use a Workspace API token"

// hint is the advice that follows ErrOrgTokenNoWorkspace, built from every way
// the command takes: "so pass --workspace, run it in a project linked to a
// Workspace, or give the token's ID".
func (a WorkspaceAdvice) hint() string {
	var ways []string
	for _, w := range adviceFor {
		if a&w.way != 0 {
			ways = append(ways, w.text)
		}
	}
	switch len(ways) {
	case 0:
		return hintCurrent
	case 1:
		return "so " + ways[0]
	case 2:
		return "so " + ways[0] + " or " + ways[1]
	}
	return "so " + strings.Join(ways[:len(ways)-1], ", ") + ", or " + ways[len(ways)-1]
}

// ErrNoWorkspace is a command that needs a Workspace, run on a saved login
// whose current Workspace is none and given none.
var ErrNoWorkspace = errors.New("no Workspace is selected: pass --workspace, or switch to one with astro workspace switch")

// ErrOrgTokenNoWorkspace is a command that needs a Workspace, run with an
// ASTRO_API_TOKEN and given none. A token supplies its Workspace itself (a
// Workspace or Deployment token) or has none (an Organization token), and a
// run with a token never takes the one the person's saved login last used.
var ErrOrgTokenNoWorkspace = errors.New("no Workspace is selected: the API token in ASTRO_API_TOKEN selects none (an Organization token does not)")

// noWorkspaceError is ErrOrgTokenNoWorkspace worded for the kind of token the
// run has, so the cause it names is the token's own. It is
// ErrOrgTokenNoWorkspace for errors.Is whatever the kind.
type noWorkspaceError struct {
	kind string
	hint string
}

func (e *noWorkspaceError) Error() string {
	return fmt.Sprintf("no Workspace is selected: the API token in ASTRO_API_TOKEN selects none (%s does not), %s", e.kind, e.hint)
}

func (e *noWorkspaceError) Is(target error) bool { return target == ErrOrgTokenNoWorkspace }

// WorkspaceDisagreeError is a run whose API token is for one Workspace and
// whose --workspace, project link or request path names another. It is a usage
// error found before any request: the token can act only in its own Workspace,
// so the request would be refused, and which of the two the person meant is
// not something to guess. 1.x let the flag win and failed at the API.
type WorkspaceDisagreeError struct {
	// Deployment is true for a Deployment token, false for a Workspace token.
	Deployment bool
	// Claim is the Workspace the token is for, Named the one the run named.
	Claim, Named string
	// Source is where Named came from.
	Source Source
}

func (e *WorkspaceDisagreeError) Error() string {
	token := "a Workspace token for " + e.Claim
	if e.Deployment {
		token = "a Deployment token for Workspace " + e.Claim
	}
	var fix string
	switch e.Source {
	case SourceFlag:
		fix = "drop --workspace, or use a token for " + e.Named
	case SourceProject:
		fix = "run outside that project, or use a token for " + e.Named
	case SourceNone, SourceToken, SourceContext, SourcePath:
		fix = "use a token for " + e.Named
	}
	return fmt.Sprintf("ASTRO_API_TOKEN is %s, but %s names %s: %s", token, e.Source.From(), e.Named, fix)
}

// IsUsageError marks WorkspaceDisagreeError as a usage error for
// cliout.IsUsage, which this package cannot import.
func (e *WorkspaceDisagreeError) IsUsageError() bool { return true }

// OmitsHelpPointer keeps the pointer to help off the error: it says what to
// change, which the command's help does not.
func (e *WorkspaceDisagreeError) OmitsHelpPointer() bool { return true }

// CheckWorkspace is nil unless the run holds a Workspace or Deployment token
// (a workspaceId claim) for a Workspace other than named, the one a person
// named through source: the flag, the project's link, or a path. A command
// family that reads a Workspace without Resolve (the core tree's manifest
// link) calls it with what it read. An Organization token has no claim, and a
// run with no token has nothing to disagree with.
func CheckWorkspace(named string, source Source) error {
	if named == "" {
		return nil
	}
	id := claimsOf()
	if id.Workspace == "" || id.Workspace == named {
		return nil
	}
	return &WorkspaceDisagreeError{Deployment: id.Deployment != "", Claim: id.Workspace, Named: named, Source: source}
}

// Request is what a command knows about the Workspace and Deployment it acts on.
type Request struct {
	// Workspace is --workspace, "" when not given.
	Workspace string
	// Project is the Workspace the project link names, "" for none or for a
	// command that does not follow the project.
	Project string
	// Deployment is --deployment's id, "" when not given.
	Deployment string
	// WantDeployment asks for the Deployment too: Target.Deployment is the
	// flag's, else a Deployment token's. A caller that wants only the
	// Workspace does not pay for reading the token.
	WantDeployment bool
	// Advice is the ways the command takes a Workspace, for the advice a
	// missing one gets. It changes no Workspace.
	Advice WorkspaceAdvice
	// Login is the login whose selection is read, for a caller handed one;
	// nil is the current context. It is read as the run's own (a token's
	// claim, or a saved login's Workspace), whatever Domain says.
	Login *config.Context
	// Domain is the host the command runs on, "" for the current context's.
	// A saved login's Workspace on another host is another login's. It does
	// not limit an API token's claims: they apply on the token run's host
	// (see TokenHost), which is the host the command runs on.
	Domain string
	// LoginError makes a login that cannot be read (none at all) the error of a
	// missing Workspace, rather than the "no Workspace" one, for a command that
	// has always said so.
	LoginError bool
	// NoSaved is what a saved login with no current Workspace says, for a
	// command with its own words for it. ErrNoWorkspace when nil.
	NoSaved error
}

// Target is the Workspace (and Deployment) a run acts on, and where each came from.
type Target struct {
	// Workspace is the Workspace, "" when no source named one.
	Workspace string
	// WorkspaceSource is where it came from, SourceNone when Workspace is "".
	WorkspaceSource Source
	// Deployment is the Deployment named by the flag or by a Deployment-scoped
	// token, "" for neither. A token's is the one Deployment it can act on, so a
	// run that names none acts on it.
	Deployment string
	// DeploymentSource is SourceFlag, SourceToken or SourceNone.
	DeploymentSource Source

	advice  WorkspaceAdvice
	noSaved error
	// readErr is why the login could not be read, when the request asked.
	readErr error
	token   bool
	// disagree is the flag's or the project's Workspace refused against the
	// token's own (see WorkspaceDisagreeError).
	disagree error
}

// Resolve decides the run's Workspace and Deployment. It reads the current
// context and ASTRO_API_TOKEN, and never prompts or calls the API.
func Resolve(r Request) Target { //nolint:gocritic // read once, as a value
	t := Target{advice: r.Advice, noSaved: r.NoSaved, token: HasAPIToken()}

	switch {
	case r.Workspace != "":
		t.Workspace, t.WorkspaceSource = r.Workspace, SourceFlag
	case r.Project != "":
		t.Workspace, t.WorkspaceSource = r.Project, SourceProject
	default:
		var err error
		t.Workspace, t.WorkspaceSource, err = fromLogin(r.Login, r.Domain, t.token)
		if r.LoginError {
			t.readErr = err
		}
	}

	// The flag, then the project, won above before the token's claim was
	// considered. A Workspace the token cannot act in is refused here, not
	// obeyed and left for the API to turn down.
	if t.token && (t.WorkspaceSource == SourceFlag || t.WorkspaceSource == SourceProject) {
		t.disagree = CheckWorkspace(t.Workspace, t.WorkspaceSource)
	}

	switch {
	case r.Deployment != "":
		t.Deployment, t.DeploymentSource = r.Deployment, SourceFlag
	case r.WantDeployment:
		// A Deployment token's claim applies on the token run's host; with no
		// token there is none.
		if id := TokenDeployment(); id != "" {
			t.Deployment, t.DeploymentSource = id, SourceToken
		}
	}
	if logger.Enabled(slog.LevelDebug) {
		logger.Debugf("%s", t.trace())
	}
	return t
}

// trace is the one line a debug log gives for a resolution: which Workspace
// and Deployment the run acts on, and where each came from.
func (t Target) trace() string { //nolint:gocritic // a small value type
	return fmt.Sprintf("workspace %s (%s), deployment %s (%s)",
		cmp.Or(t.Workspace, "none"), cmp.Or(t.WorkspaceSource.Label(flagWorkspace), t.WorkspaceSource.name()),
		cmp.Or(t.Deployment, "none"), cmp.Or(t.DeploymentSource.Label("--deployment"), t.DeploymentSource.name()))
}

// name is the source for a log, where Label has none to give.
func (s Source) name() string {
	if s == SourceContext {
		return "the saved login"
	}
	return "no source"
}

// fromLogin is the Workspace of the login the run is on, and which kind of
// source that is. The error is a login that cannot be read at all, which names
// no Workspace.
func fromLogin(login *config.Context, domain string, token bool) (string, Source, error) {
	var ws string
	switch {
	case token:
		// An API token's claim is the run's selection, read from the token
		// itself. No context is read, so the saved login's Workspace is out of
		// reach whether or not setup overlaid the token on it, and a machine
		// with no login at all still has a selection.
		ws = claimsOf().Workspace
	case login != nil:
		ws = login.Workspace
	case domain == "":
		ctx, err := config.GetCurrentContext()
		if err != nil {
			return "", SourceNone, err
		}
		ws = ctx.Workspace
	default:
		ctx, err := (&config.Context{Domain: domain}).GetContext()
		if err != nil {
			return "", SourceNone, nil
		}
		ws = ctx.Workspace
	}
	switch {
	case ws == "":
		return "", SourceNone, nil
	case token:
		return ws, SourceToken, nil
	}
	return ws, SourceContext, nil
}

// TokenHost is the Astro host a run with ASTRO_API_TOKEN uses, for every
// lookup it makes (Deployments, Environment Manager, the Workspace's
// objects): the host the command names on its command line (explicit), else
// the project manifest's [tool.astro] domain (project), else ASTRO_DOMAIN,
// else astronomer.io. Each is normalized as `astro login` stores a host
// ("https://cloud.astronomer-dev.io/" is "astronomer-dev.io").
//
// The saved context is not in the list: a token run takes nothing from the
// saved login, its host included, so a machine switched to dev by an earlier
// `astro login` does not send a production token to dev. A token is one
// credential for one host, and the run must name it. A run with no token does
// not use this rule.
func TokenHost(explicit, project string) string {
	for _, d := range []string{explicit, project, os.Getenv("ASTRO_DOMAIN")} {
		if h := manifest.NormalizeDomain(d); h != "" {
			return h
		}
	}
	return manifest.DefaultWorkspaceDomain
}

// Found reports whether a source named a Workspace.
func (t Target) Found() bool { //nolint:gocritic // a small value type
	return t.Workspace != ""
}

// Disagrees is the error for a Workspace named by the flag or the project that
// is not the one the run's Workspace or Deployment token is for, nil when they
// agree or the run has no such token (see WorkspaceDisagreeError). A caller
// that reads Workspace without calling Require checks it first.
func (t Target) Disagrees() error { //nolint:gocritic // a small value type
	return t.disagree
}

// Require is nil when the run has a Workspace, else the error for its absence
// (Missing), or the one for a Workspace its token cannot act in (Disagrees). A
// command that asks a question or calls the API calls it first, so nothing is
// asked on the way to this error.
func (t Target) Require() error { //nolint:gocritic // a small value type
	if t.disagree != nil {
		return t.disagree
	}
	if t.Found() {
		return nil
	}
	return t.Missing()
}

// Missing is the error for a run with no Workspace: under an API token,
// ErrOrgTokenNoWorkspace with the advice the command's flags and project
// support, else what a saved login without one says.
func (t Target) Missing() error { //nolint:gocritic // a small value type
	if t.readErr != nil {
		return t.readErr
	}
	if t.token {
		// An unusable token names nothing because it is unusable, not because
		// it is an Organization token: say so.
		id, _, err := TokenIdentity()
		if err != nil {
			return err
		}
		if id.Workspace != "" {
			// A Workspace or Deployment token names its Workspace, so Resolve
			// never leaves it without one; a Target built by hand does not get
			// a message that blames the token's kind.
			return fmt.Errorf("no Workspace is selected, though the API token in ASTRO_API_TOKEN names Workspace %s", id.Workspace)
		}
		kind := "an Organization token"
		if id.Deployment != "" {
			// It names no Workspace either, which a genuine Deployment token
			// does: do not call it an Organization token.
			kind = "a Deployment token with no workspaceId claim"
		}
		return &noWorkspaceError{kind: kind, hint: t.advice.hint()}
	}
	if t.noSaved != nil {
		return t.noSaved
	}
	return ErrNoWorkspace
}

// TokenDeployment is the Deployment ASTRO_API_TOKEN is scoped to: the id of
// its one deploymentId permission, or "" for no token, or a token with none
// (a Workspace or Organization token), or with several, which is not scoped to
// one Deployment. It is read from the token each time it is asked, so no run
// state stands in for it.
func TokenDeployment() string {
	return claimsOf().Deployment
}

// RequireWorkspace is the Workspace of a command that takes it from
// --workspace (flag, "" when not given) or else the run's own selection, and
// has no project link or other way to name one: the Workspace, or the error
// for there being none.
func RequireWorkspace(flag string) (string, error) {
	t := Resolve(Request{Workspace: flag, Advice: ByFlag, LoginError: true})
	if err := t.Require(); err != nil {
		return "", err
	}
	return t.Workspace, nil
}
