package ide

import (
	httpContext "context"
	"fmt"
	"io"
	"net/http"

	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
)

// SessionExporter streams a session's export. The generated *WithResponse
// call reads the whole archive into memory first, so an import takes this,
// the plain call, which astrov1alpha1.ClientInterface (and so the
// *astrov1alpha1.ClientWithResponses the CLI builds) has.
type SessionExporter interface {
	ExportAstroIdeSessionTar(ctx httpContext.Context, organizationID, workspaceID, projectID, sessionID string, params *astrov1alpha1.ExportAstroIdeSessionTarParams, reqEditors ...astrov1alpha1.RequestEditorFn) (*http.Response, error)
}

// maxErrorBody caps what is read of a failed export's answer, for its message.
const maxErrorBody = 1 << 20

// downloadSession streams a session's export into dst, failing past what an
// import reads.
func downloadSession(ctx httpContext.Context, exporter SessionExporter, organizationID, workspaceID, projectID, sessionID string, dst io.Writer) error {
	resp, err := exporter.ExportAstroIdeSessionTar(ctx, organizationID, workspaceID, projectID, sessionID, &astrov1alpha1.ExportAstroIdeSessionTarParams{})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody)) //nolint:errcheck // the status is the error; the body only words it
		if err := astrov1alpha1.NormalizeAPIError(resp, body); err != nil {
			return err
		}
		// A status the API calls success but that carries no archive (204).
		return fmt.Errorf("the server returned no archive (%d)", resp.StatusCode)
	}
	_, err = copyLimited(dst, resp.Body)
	return err
}

// copyLimited copies src to dst, failing with errArchiveTooLarge past what an
// import reads.
func copyLimited(dst io.Writer, src io.Reader) (int64, error) {
	return io.Copy(dst, newLimitedReader(src, maxImportBytes+archiveOverhead))
}
