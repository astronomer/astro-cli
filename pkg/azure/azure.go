package azure

import (
	"context"
	"io"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

type DagClient struct {
	BlobClient *azblob.BlockBlobClient
}

type Azure interface {
	Upload(ac DagClient, dagFileReader io.Reader) (string, error)
	CreateSASDagClient(sasLink string) (DagClient, error)
}

type ClientAPI interface {
	NewBlockBlobClientWithNoCredential(blobURL string, options *azblob.ClientOptions) (*azblob.BlockBlobClient, error)
	UploadStream(ctx context.Context, body io.Reader, o *azblob.UploadStreamOptions) (*azblob.BlockBlobCommitBlockListResponse, error)
}

func CreateSASDagClient(sasLink string) (DagClient, error) {
	blobClient, err := azblob.NewBlockBlobClientWithNoCredential(sasLink, nil)
	if err != nil {
		return DagClient{}, err
	}

	return DagClient{BlobClient: blobClient}, nil
}

func Upload(ac DagClient, dagFileReader io.Reader) (string, error) {
	uploadRes, err := ac.BlobClient.UploadStream(context.TODO(), dagFileReader, azblob.UploadStreamOptions{})
	if err != nil {
		return "", err
	}
	versionID := *uploadRes.VersionID

	return versionID, nil
}
