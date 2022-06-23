package azure

import (
	"context"
	// "fmt"
	// "os"
	"io"
	// "log"

	// "github.com/pkg/errors"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	// "github.com/astronomer/astro-cli/pkg/fileutil"
)

type azureDagClient struct {
	blobClient *azblob.BlockBlobClient
}

func upload(blobClient *azblob.BlockBlobClient, uploadFileReader io.Reader) error {
	_, err := blobClient.UploadStream(context.TODO(), uploadFileReader, azblob.UploadStreamOptions{})
	if err != nil {
		return err
	}

	// log.Print("upload file to blob ", blobClient.URL())
	return nil
}

func getBlockBlobClientFromSAS(blobUrl string) (*azblob.BlockBlobClient, error) {
	blobClient, err := azblob.NewBlockBlobClientWithNoCredential(blobUrl, nil)
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

func (ac azureDagClient) Upload(dagFileReader io.Reader) error {
	return upload(ac.blobClient, dagFileReader)
}

// func (ac AzureDagClient) Upload(url, path string) (bool, error) {
// func Upload(url, path string) (bool, error) {
// 	// blobClient := ac.blobClient

// 	// serviceClient, err := blobClient.NewServiceClientWithNoCredential(url, nil)
// 	// if err != nil {
// 	// 	return err
// 	// }

// 	// dagClient := ac.blobClient

// 	// fmt.Println(dagClient)
// 	// blockBlobClient := dagClient.NewBlockBlobURL(url, dagClient.NewPipeline(dagClient.NewAnonymousCredential(), dagClient.PipelineOptions{}))
// 	blobClient, err := azblob.NewBlockBlobClientWithNoCredential(url, nil)
// 	fmt.Println(blobClient)
// 	// ctx := context.Background()

// 	fileExist, err := fileutil.Exists(path, nil)
// 	if err != nil {
// 		return false, errors.Wrapf(err, "failed to find tar ball '%s'", path)
// 	}

// 	if fileExist {
// 		file, err := os.Open(path)
// 		if err != nil {
// 			return false, err
// 		}
// 		// _, err = blobClient.UploadAsync(ctx, file, nil)
// 		// if err != nil {
// 		// 	return false, err
// 		// }
	
// 		return true, nil
// 	}

// 	return false, nil
// }