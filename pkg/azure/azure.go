package azure

import (
	"context"
	"io"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

type Azure interface {
	Upload(sasLink string, dagFileReader io.Reader) (string, error)
}

func Upload(sasLink string, dagFileReader io.Reader) (string, error) {
	blobClient, err := azblob.NewBlockBlobClientWithNoCredential(sasLink, nil)
	if err != nil {
		return "", err
	}
	uploadRes, err := blobClient.UploadStream(context.TODO(), dagFileReader, azblob.UploadStreamOptions{})
	if err != nil {
		return "", err
	}
	versionID := *uploadRes.VersionID

	return versionID, nil
}
