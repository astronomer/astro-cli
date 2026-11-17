package deployment

import (
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

// fetchedClient is a client that carries a Deployment its holder already read
// by id. Everything but the lookup of that Deployment goes to the client it
// wraps.
type fetchedClient struct {
	astrov1.APIClient
	d astrov1.Deployment
}

// WithFetched is client, remembering d, a Deployment the caller read by its id
// to learn its Workspace. A GetDeployment through the returned client for that
// Deployment, in its Workspace, answers with d instead of listing the
// Workspace and fetching the Deployment a second time, so a Deployment given
// by id is one GET however many layers want it. Any other lookup, or another
// call, goes to client.
func WithFetched(client astrov1.APIClient, d astrov1.Deployment) astrov1.APIClient { //nolint:gocritic // a Deployment is passed by value throughout this package
	return &fetchedClient{APIClient: client, d: d}
}

// fetchedFor is the Deployment id, in workspace ws, that client was handed
// with WithFetched, and whether it was.
func fetchedFor(client astrov1.APIClient, ws, id string) (astrov1.Deployment, bool) {
	f, ok := client.(*fetchedClient)
	if !ok || f.d.Id != id || f.d.WorkspaceId != ws {
		return astrov1.Deployment{}, false
	}
	return f.d, true
}
