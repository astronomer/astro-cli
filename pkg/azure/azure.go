package azure

import (
	"context"
	"io"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

type azureDagClient struct {
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

func CreateSASDagClient(sasLink string) (azureDagClient, error) {
	blobClient, err := getBlockBlobClientFromSAS(sasLink)
	if err != nil {
		return azureDagClient{}, err
	}
	return azureDagClient{blobClient: blobClient}, nil
}

func (ac azureDagClient) Upload(dagFileReader io.Reader) (string, error) {
	return upload(ac.blobClient, dagFileReader)
}
