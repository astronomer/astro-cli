package azure

import (
	"context"
	"io"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

type DagClient struct {
	blobClient *azblob.BlockBlobClient
}

func upload(blobClient *azblob.BlockBlobClient, uploadFileReader io.Reader) (string, error) {
	uploadRes, err := blobClient.UploadStream(context.TODO(), uploadFileReader, azblob.UploadStreamOptions{})
	if err != nil {
		return "", err
	}

	return *uploadRes.VersionID, nil
}

func getBlockBlobClientFromSAS(blobURL string) (*azblob.BlockBlobClient, error) {
	blobClient, err := azblob.NewBlockBlobClientWithNoCredential(blobURL, nil)
	if err != nil {
		return nil, err
	}
	return blobClient, nil
}

func CreateSASDagClient(sasLink string) (DagClient, error) {
	blobClient, err := getBlockBlobClientFromSAS(sasLink)
	if err != nil {
		return DagClient{}, err
	}
	return DagClient{blobClient: blobClient}, nil
}

func (ac DagClient) Upload(dagFileReader io.Reader) (string, error) {
	return upload(ac.blobClient, dagFileReader)
}
