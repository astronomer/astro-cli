package emenv

import (
	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/internal/envresolve"
)

// NewProvider builds the workspace Environment Manager provider for a run: it
// resolves `source = "workspace"` names against workspaceID's objects. reveal
// asks for secret values (start and get); list passes reveal = false so it
// reads presence only and never pulls a secret value. One provider serves the
// whole run, so its fetch is shared across every workspace-source name.
//
// An empty workspaceID means the manifest sets no top-level `workspace`, which
// a workspace source needs: the provider is then unavailable and names the fix.
func NewProvider(workspaceID string, client astrov1.APIClient, reveal bool) envresolve.Provider {
	if workspaceID == "" {
		return Unavailable("the manifest sets no `workspace`; add `workspace = \"<id>\"` under [tool.astro]")
	}
	return &provider{workspaceID: workspaceID, client: client, reveal: reveal}
}

// Unavailable returns a provider that is absent for the given reason. Stage 1
// uses it for `astro local start --docker`, which cannot inject a resolved
// value without writing it into the on-disk compose file — the one thing the
// read-through posture rules out. A workspace-source name then resolves from
// nowhere and, if required, gates the start with the reason.
func Unavailable(reason string) envresolve.Provider {
	return &unavailable{reason: reason}
}

// unavailable is a provider that never resolves and says why.
type unavailable struct{ reason string }

func (u *unavailable) Lookup(string) (string, bool) { return "", false }
func (u *unavailable) Label() string                { return sourceLabel + " (unavailable: " + u.reason + ")" }
func (u *unavailable) Diagnose(string) string       { return u.reason }
