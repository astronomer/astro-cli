package deployment

import (
	"github.com/astronomer/astro-cli/internal/astrosession"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

// sourcedClient is a client that carries where its holder took a Workspace
// from, so a lookup in that Workspace that fails says so. Everything else goes
// to the client it wraps.
type sourcedClient struct {
	astrov1.APIClient
	ws  string
	src astrosession.Source
}

// WithWorkspaceSource is client, remembering that the run took workspace ws
// from src (the flag, the project link, the token's claim). A lookup through
// the returned client, in ws, words the errors it builds about ws with src
// (see sourceFor); a lookup in any other Workspace does not, so a message about
// another Workspace is never labeled with this one's source.
func WithWorkspaceSource(client astrov1.APIClient, ws string, src astrosession.Source) astrov1.APIClient {
	return &sourcedClient{APIClient: client, ws: ws, src: src}
}

// sourceFor is where the run took ws from, as client was told with
// WithWorkspaceSource, and SourceNone for any other Workspace or a client that
// was told nothing.
func sourceFor(client astrov1.APIClient, ws string) astrosession.Source {
	for client != nil {
		switch c := client.(type) {
		case *sourcedClient:
			if c.ws == ws {
				return c.src
			}
			return astrosession.SourceNone
		case *fetchedClient:
			client = c.APIClient
		default:
			return astrosession.SourceNone
		}
	}
	return astrosession.SourceNone
}
