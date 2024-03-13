package cmd

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	askastro "github.com/astronomer/astro-cli/ask-astro"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	"github.com/spf13/cobra"
)

func newDAGReviewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dag-review DAG-ID",
		Short: "Use Ask Astro to review our DAG for errors, bad code, and DAG best practices",
		Long:  "Use Ask Astro to review our DAG for errors, bad code, and DAG best practices",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE: utils.EnsureProjectDir,
		RunE:    dagReview,
	}

	return cmd
}

func dagReview(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	var (
		dagFilePath string
		err         error
	)

	if len(args) > 0 {
		dagFile = args[0]
	}

	dagsFolder := config.WorkingPath + "/dags/"

	if dagFile == "" {
		dagFilePath, err = getFirstDAGFile(dagsFolder)
		if err != nil {
			return err
		}
	} else if !strings.Contains(dagFile, dagsFolder) {
		dagFilePath = dagsFolder + dagFile
	} else {
		dagFilePath = dagFile
	}

	return askastro.DAGReview(dagFilePath, false)
}

func getFirstDAGFile(folderName string) (string, error) {
	files, err := ioutil.ReadDir(folderName)
	if err != nil {
		return "", err
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		dagFilePath := filepath.Join(folderName, file.Name())
		if strings.Contains(dagFilePath, ".py") {
			return dagFilePath, nil
		}
	}

	return "", fmt.Errorf("no DAG files found in folder: %s", folderName)
}
