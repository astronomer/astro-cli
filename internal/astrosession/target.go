package astrosession

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/util"
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
//     under ASTRO_API_TOKEN, and never another host's;
//  5. the legacy ASTRONOMER_KEY_* rule: the Workspace of the organization's
//     first Deployment, which setup records for such a run.
//
// A run with ASTRO_API_TOKEN or ASTRONOMER_KEY_* has its own overlay on the
// current context (cmd/astro/setup.go records the claim or the first
// Deployment's Workspace there, for the process only), so the context read in
// steps 3 to 5 is that overlay and the saved login's Workspace is out of
// reach: the source names which of the three it is.
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
	// SourceLegacyKey is the Workspace of the first Deployment of the
	// organization an ASTRONOMER_KEY_ID run belongs to.
	SourceLegacyKey
)

func (s Source) String() string {
	switch s {
	case SourceFlag:
		return "flag"
	case SourceProject:
		return "project"
	case SourceToken:
		return "token"
	case SourceContext:
		return "context"
	case SourceLegacyKey:
		return "legacy key"
	}
	return "none"
}

// Accepts is what a command lets a person name a Workspace with, besides the
// token's claim and the saved login. It words the advice a missing Workspace
// gets, so it never offers a way the command would not take.
type Accepts uint8

const (
	// ByFlag is --workspace. Every command has it.
	ByFlag Accepts = 1 << iota
	// ByProject is a project linked to a Workspace, for the command families
	// that follow the project.
	ByProject
	// ByID is the id of the token or Deployment the command acts on, which
	// identifies it with no Workspace to look in.
	ByID
	// ByPath is writing the Workspace's id where the command takes a path (the
	// astro api commands' placeholders).
	ByPath
)

// The advice that follows ErrOrgTokenNoWorkspace, by what the command takes.
const (
	hintFlag          = "so pass --workspace"
	hintFlagOrProject = "so pass --workspace, or run in a project linked to one"
	// hintFlagOrID is for a command that finds a token in the Workspace,
	// unless the token is given by ID.
	hintFlagOrID = "so pass --workspace or the token's ID, or run in a project linked to one" //nolint:gosec // advice, not a credential
	hintPath = "so write the id in the path"
	// hintCurrent is for a command that acts on the current Workspace alone,
	// which a run with an API token does not have.
	hintCurrent = "so use a Workspace API token"
)

func (a Accepts) hint() string {
	switch {
	case a&ByPath != 0:
		return hintPath
	case a&ByID != 0:
		return hintFlagOrID
	case a&ByProject != 0:
		return hintFlagOrProject
	case a&ByFlag != 0:
		return hintFlag
	}
	return hintCurrent
}

// ErrNoWorkspace is a command that needs a Workspace, run on a saved login
// whose current Workspace is none and given none.
var ErrNoWorkspace = errors.New("no Workspace is selected: pass --workspace, or switch to one with astro workspace switch")

// ErrOrgTokenNoWorkspace is a command that needs a Workspace, run with an
// ASTRO_API_TOKEN and given none. A token supplies its Workspace itself (a
// Workspace or Deployment token) or has none (an Organization token), and a
// run with a token never takes the one the person's saved login last used.
var ErrOrgTokenNoWorkspace = errors.New("no Workspace is selected: the API token in ASTRO_API_TOKEN selects none (an Organization token does not)")

// Request is what a command knows about the Workspace and Deployment it acts on.
type Request struct {
	// Workspace is --workspace, "" when not given.
	Workspace string
	// Project is the Workspace the project link names, "" for none or for a
	// command that does not follow the project.
	Project string
	// Deployment is --deployment's id, "" when not given.
	Deployment string
	// Accepts is what the command takes, for the advice.
	Accepts Accepts
	// Login is the login whose selection is read, for a caller handed one;
	// nil is the current context. It is read as the run's own (a token's
	// claim, or a saved login's Workspace), whatever Domain says.
	Login *config.Context
	// Domain is the host the command runs on, "" for the current context's.
	// An API token's claim is the run's selection on the current host only,
	// and a saved login's Workspace on another host is another login's.
	Domain string
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

	accepts Accepts
	noSaved error
	// readErr is why the login could not be read, when it could not.
	readErr error
	token   bool
}

// Resolve decides the run's Workspace and Deployment. It reads the current
// context and ASTRO_API_TOKEN, and never prompts or calls the API.
func Resolve(r Request) Target {
	t := Target{accepts: r.Accepts, noSaved: r.NoSaved, token: HasAPIToken()}
	onCurrent := r.Domain == "" || sameDomain(r.Domain, currentDomain())

	switch {
	case r.Workspace != "":
		t.Workspace, t.WorkspaceSource = r.Workspace, SourceFlag
	case r.Project != "":
		t.Workspace, t.WorkspaceSource = r.Project, SourceProject
	default:
		t.Workspace, t.WorkspaceSource, t.readErr = fromLogin(r.Login, r.Domain, t.token, onCurrent)
	}

	switch {
	case r.Deployment != "":
		t.Deployment, t.DeploymentSource = r.Deployment, SourceFlag
	case onCurrent:
		if id := TokenDeployment(); id != "" {
			t.Deployment, t.DeploymentSource = id, SourceToken
		}
	}
	return t
}

// fromLogin is the Workspace of the login the run is on, and which kind of
// source that is. The error is a login that could not be read at all.
func fromLogin(login *config.Context, domain string, token, onCurrent bool) (string, Source, error) {
	var ws string
	switch {
	case login != nil:
		ws = login.Workspace
	case domain == "" || (token && onCurrent):
		// An API token's selection is overlaid on the current context.
		ctx, err := config.GetCurrentContext()
		if err != nil {
			return "", SourceNone, err
		}
		ws = ctx.Workspace
	case token:
		// Another host's saved Workspace is not one this run chose.
		return "", SourceNone, nil
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
	case os.Getenv("ASTRONOMER_KEY_ID") != "":
		return ws, SourceLegacyKey, nil
	}
	return ws, SourceContext, nil
}

func currentDomain() string {
	d, err := config.GetCurrentDomain()
	if err != nil {
		return ""
	}
	return d
}

func sameDomain(a, b string) bool {
	return strings.EqualFold(strings.ReplaceAll(a, "_", "."), strings.ReplaceAll(b, "_", "."))
}

// Found reports whether a source named a Workspace.
func (t Target) Found() bool { return t.Workspace != "" }

// Require is nil when the run has a Workspace, else the error for its absence
// (Missing). A command that asks a question or calls the API calls it first,
// so nothing is asked on the way to this error.
func (t Target) Require() error {
	if t.Found() {
		return nil
	}
	return t.Missing()
}

// Missing is the error for a run with no Workspace: under an API token,
// ErrOrgTokenNoWorkspace with the advice the command's flags and project
// support, else what a saved login without one says. A login that could not be
// read is that error.
func (t Target) Missing() error {
	if t.readErr != nil {
		return t.readErr
	}
	if t.token {
		return fmt.Errorf("%w, %s", ErrOrgTokenNoWorkspace, t.accepts.hint())
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
	stored, ok := APIToken()
	if !ok {
		return ""
	}
	claims, err := util.ParseAPIToken(Credential(stored))
	if err != nil || claims == nil {
		return ""
	}
	id := ""
	for _, permission := range claims.Permissions {
		v, ok := strings.CutPrefix(permission, "deploymentId:")
		if !ok {
			continue
		}
		if id != "" || v == "" {
			return ""
		}
		id = v
	}
	return id
}

// RequireWorkspace is the Workspace of a command that takes it from
// --workspace (flag, "" when not given) or else the run's own selection, and
// has no project link or other way to name one: the Workspace, or the error
// for there being none.
func RequireWorkspace(flag string) (string, error) {
	t := Resolve(Request{Workspace: flag, Accepts: ByFlag})
	if err := t.Require(); err != nil {
		return "", err
	}
	return t.Workspace, nil
}
