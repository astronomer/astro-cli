package registry

import (
	"archive/zip"
	"errors"
	"fmt"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func newRegistryInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "init [URL]",
		Aliases: []string{"i"},
		Short:   "Download a project template from the Astronomer Registry",
		Long:    "Download a project from the Astronomer Registry as a local Astro project.",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				projectUrl := args[0]
				return initProject(projectUrl, projectBranch)
			} else {
				return errors.New("too many arguments")
			}
		},
	}
	cmd.Flags().StringVar(&projectBranch, "branch", "main", "GitHub Project Branch or Tag. Optional.")
	return cmd
}

func addProviderExampleDag(name string) {
	// TODO - this doesn't exist yet
	logrus.Fatal("THIS DOESN'T EXIST YET")
	if false {
		downloadDag(name, "latest", false)
	}
}

func initProject(projectUrl string, branch string) error {
	parsedProjectUrl, err := url.Parse(projectUrl)
	printutil.LogFatal(err)

	//project := filepath.Base(parsedProjectUrl.Path)

	zipUrl := parsedProjectUrl.JoinPath(fmt.Sprintf("archive/refs/heads/%s.zip", branch))
	zipUrlString := zipUrl.String()
	zipFileName := filepath.Base(zipUrl.Path)
	httputil.DownloadResponseToFile(zipUrlString, zipFileName)

	err = UnzipAndMergeInPlace(zipFileName, ".")
	printutil.LogFatal(err)
	return nil
}

func UnzipAndMergeInPlace(src, dest string) error {
	// Adapted from:: https://stackoverflow.com/a/24792688
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Close(); err != nil {
			panic(err)
		}
	}()

	err = os.MkdirAll(dest, 0755)
	if err != nil {
		return err
	}

	// Closure to address file descriptors issue with all the deferred .Close() methods
	extractAndWriteFile := func(f *zip.File) error {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() {
			if err := rc.Close(); err != nil {
				panic(err)
			}
		}()

		path := filepath.Join(dest, f.Name)

		// Check for ZipSlip (Directory traversal)
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", path)
		}

		if f.FileInfo().IsDir() {
			err = os.MkdirAll(path, f.Mode())
			if err != nil {
				return err
			}
		} else {
			err = os.MkdirAll(filepath.Dir(path), f.Mode())
			if err != nil {
				return err
			}
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer func() {
				if err := f.Close(); err != nil {
					panic(err)
				}
			}()

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
		return nil
	}

	for _, f := range r.File {
		err := extractAndWriteFile(f)
		if err != nil {
			return err
		}
	}
	return nil
}
