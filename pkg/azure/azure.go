package azure

import (
	"context"
	"io"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
)

type Azure interface {
	Upload(sasLink string, dagFileReader io.Reader) (string, error)
}

func Upload(sasLink string, dagFileReader io.Reader) (string, error) {
	blobClient, err := blockblob.NewClientWithNoCredential(sasLink, nil)
	if err != nil {
		return "", err
	}
	uploadRes, err := blobClient.UploadStream(context.TODO(), dagFileReader, &blockblob.UploadStreamOptions{})
	if err != nil {
		return "", err
	}
	versionID := *uploadRes.VersionID

	return versionID, nil
}
