package azure

import (
	"context"
	"io"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
)

type Azure interface {
	Upload(ctx context.Context, sasLink string, dagFileReader io.Reader) (string, error)
}

// Upload streams dagFileReader to the blob at sasLink. ctx ends the upload,
// so an interrupt aborts a large one at once.
func Upload(ctx context.Context, sasLink string, dagFileReader io.Reader) (string, error) {
	blobClient, err := blockblob.NewClientWithNoCredential(sasLink, nil)
	if err != nil {
		return "", err
	}
	uploadRes, err := blobClient.UploadStream(ctx, dagFileReader, &blockblob.UploadStreamOptions{})
	if err != nil {
		return "", err
	}
	versionID := *uploadRes.VersionID

	return versionID, nil
}
