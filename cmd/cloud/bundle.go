package cloud

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/astronomer/astro-cli/cloud/bundle"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/config"
	"github.com/spf13/cobra"
)

var (
	bundleDeploymentID   string
	bundleDeploymentName string
	bundleWorkspaceID    string
	bundleRemotePath        string
	bundleFileOutputPath    string
	bundleArchiveOutputPath string
	bundleDestination       string
	bundleSyncPath       string
	bundleTargetPath     string
	bundleNoOverwrite    bool

	// function vars for testability
	BundleListFiles     = bundle.ListFiles
	BundleUploadFile    = bundle.UploadFile
	BundleDownloadFile  = bundle.DownloadFile
	BundleDeleteFile    = bundle.DeleteFile
	BundleMoveFile      = bundle.MoveFile
	BundleDuplicateFile = bundle.DuplicateFile
	BundleSync          = bundle.Sync
	BundleUploadArchive = bundle.UploadArchive
	BundleDownloadArch  = bundle.DownloadArchive
)

func newDeploymentDagsCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "dags",
		Short:   "Manage deployment DAG bundle files",
	}
	cmd.PersistentFlags().StringVar(&bundleDeploymentID, "deployment-id", "", "Deployment ID")
	cmd.PersistentFlags().StringVarP(&bundleDeploymentName, "deployment-name", "n", "", "Name of the Deployment")
	cmd.PersistentFlags().StringVar(&bundleWorkspaceID, "workspace-id", "", "Workspace for your Deployment")
	cmd.AddCommand(
		newBundleSyncCmd(out),
		newBundleFileListCmd(out),
		newBundleFileUploadCmd(out),
		newBundleFileDownloadCmd(out),
		newBundleFileDeleteCmd(out),
		newBundleFileMoveCmd(out),
		newBundleFileDuplicateCmd(out),
		newBundleArchiveUploadCmd(out),
		newBundleArchiveDownloadCmd(out),
	)
	return cmd
}

// resolveBundleDeploymentID resolves a deployment ID from args, flags, or interactive prompt.
func resolveBundleDeploymentID(args []string) (string, string, error) {
	// get org ID
	c, err := config.GetCurrentContext()
	if err != nil {
		return "", "", err
	}
	orgID := c.Organization

	// deployment ID from args
	if len(args) > 0 {
		return orgID, args[0], nil
	}
	// deployment ID from flag
	if bundleDeploymentID != "" {
		return orgID, bundleDeploymentID, nil
	}

	// resolve workspace
	ws := bundleWorkspaceID
	if ws == "" {
		ws, err = coalesceWorkspace()
		if err != nil {
			return "", "", fmt.Errorf("failed to find a valid workspace: %w", err)
		}
	}

	// interactive deployment selection
	selectedDeployment, err := deployment.GetDeployment(ws, "", bundleDeploymentName, false, nil, platformCoreClient, astroCoreClient)
	if err != nil {
		return "", "", err
	}
	return orgID, selectedDeployment.Id, nil
}

// --- bundle sync ---

func newBundleSyncCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync [DEPLOYMENT-ID]",
		Short: "Tar, gzip, and upload the current directory as a bundle archive",
		Long:  "Tar and gzip a local directory (default: current directory) and upload it as a bundle archive to a Deployment.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  bundleSyncCmd,
	}
	cmd.Flags().StringVar(&bundleSyncPath, "path", ".", "Local directory to sync")
	cmd.Flags().StringVar(&bundleTargetPath, "target-path", "", "Remote target path within the bundle")
	cmd.Flags().BoolVar(&bundleNoOverwrite, "no-overwrite", false, "Do not overwrite existing files")
	return cmd
}

func bundleSyncCmd(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	orgID, deployID, err := resolveBundleDeploymentID(args)
	if err != nil {
		return err
	}
	return BundleSync(bundleStreamClient, orgID, deployID, bundleSyncPath, bundleTargetPath, !bundleNoOverwrite)
}

// --- bundle file ---

func newBundleFileListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list [DEPLOYMENT-ID]",
		Aliases: []string{"ls"},
		Short:   "List files in a deployment bundle",
		Args:    cobra.MaximumNArgs(1),
		RunE:    bundleFileListCmd(out),
	}
	cmd.Flags().StringVar(&bundleRemotePath, "path", "", "Remote path prefix to filter")
	return cmd
}

func bundleFileListCmd(out io.Writer) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		orgID, deployID, err := resolveBundleDeploymentID(args)
		if err != nil {
			return err
		}
		return BundleListFiles(out, astroCoreClient, orgID, deployID, bundleRemotePath)
	}
}

func newBundleFileUploadCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "upload [DEPLOYMENT-ID] <local-file-path>",
		Aliases: []string{"up"},
		Short:   "Upload a local file to a deployment bundle",
		Args:    cobra.RangeArgs(1, 2),
		RunE:    bundleFileUploadCmd,
	}
	cmd.Flags().StringVar(&bundleRemotePath, "remote-path", "", "Destination path within the bundle (required)")
	_ = cmd.MarkFlagRequired("remote-path")
	return cmd
}

func bundleFileUploadCmd(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	var localPath string
	var deployArgs []string
	// If 2 args: first is deployment ID, second is local path
	// If 1 arg: it's the local path (deployment resolved from flags/prompt)
	if len(args) == 2 { //nolint:mnd
		deployArgs = args[:1]
		localPath = args[1]
	} else {
		localPath = args[0]
	}

	orgID, deployID, err := resolveBundleDeploymentID(deployArgs)
	if err != nil {
		return err
	}
	return BundleUploadFile(bundleStreamClient, orgID, deployID, bundleRemotePath, localPath)
}

func newBundleFileDownloadCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "download [DEPLOYMENT-ID] <remote-path>",
		Aliases: []string{"dl"},
		Short:   "Download a file from a deployment bundle",
		Args:    cobra.RangeArgs(1, 2),
		RunE:    bundleFileDownloadCmd(out),
	}
	cmd.Flags().StringVarP(&bundleFileOutputPath, "output", "o", "", "Local output file path (default: basename of remote path)")
	return cmd
}

func bundleFileDownloadCmd(out io.Writer) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		var remotePath string
		var deployArgs []string
		if len(args) == 2 { //nolint:mnd
			deployArgs = args[:1]
			remotePath = args[1]
		} else {
			remotePath = args[0]
		}

		// Default output to the basename of the remote path
		outputPath := bundleFileOutputPath
		if outputPath == "" {
			outputPath = filepath.Base(remotePath)
		}

		orgID, deployID, err := resolveBundleDeploymentID(deployArgs)
		if err != nil {
			return err
		}
		return BundleDownloadFile(bundleStreamClient, orgID, deployID, remotePath, outputPath, out)
	}
}

func newBundleFileDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [DEPLOYMENT-ID] <remote-path>",
		Aliases: []string{"rm"},
		Short:   "Delete a file from a deployment bundle",
		Args:    cobra.RangeArgs(1, 2),
		RunE:    bundleFileDeleteCmd,
	}
	return cmd
}

func bundleFileDeleteCmd(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	var remotePath string
	var deployArgs []string
	if len(args) == 2 { //nolint:mnd
		deployArgs = args[:1]
		remotePath = args[1]
	} else {
		remotePath = args[0]
	}

	orgID, deployID, err := resolveBundleDeploymentID(deployArgs)
	if err != nil {
		return err
	}
	return BundleDeleteFile(astroCoreClient, orgID, deployID, remotePath)
}

func newBundleFileMoveCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "move [DEPLOYMENT-ID] <source-path>",
		Aliases: []string{"mv"},
		Short:   "Move a file within a deployment bundle",
		Args:    cobra.RangeArgs(1, 2),
		RunE:    bundleFileMoveCmd,
	}
	cmd.Flags().StringVar(&bundleDestination, "destination", "", "Destination path within the bundle (required)")
	_ = cmd.MarkFlagRequired("destination")
	return cmd
}

func bundleFileMoveCmd(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	var sourcePath string
	var deployArgs []string
	if len(args) == 2 { //nolint:mnd
		deployArgs = args[:1]
		sourcePath = args[1]
	} else {
		sourcePath = args[0]
	}

	orgID, deployID, err := resolveBundleDeploymentID(deployArgs)
	if err != nil {
		return err
	}
	return BundleMoveFile(astroCoreClient, orgID, deployID, sourcePath, bundleDestination)
}

func newBundleFileDuplicateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "duplicate [DEPLOYMENT-ID] <source-path>",
		Aliases: []string{"cp"},
		Short:   "Duplicate a file within a deployment bundle",
		Args:    cobra.RangeArgs(1, 2),
		RunE:    bundleFileDuplicateCmd,
	}
	cmd.Flags().StringVar(&bundleDestination, "destination", "", "Destination path within the bundle (required)")
	_ = cmd.MarkFlagRequired("destination")
	return cmd
}

func bundleFileDuplicateCmd(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	var sourcePath string
	var deployArgs []string
	if len(args) == 2 { //nolint:mnd
		deployArgs = args[:1]
		sourcePath = args[1]
	} else {
		sourcePath = args[0]
	}

	orgID, deployID, err := resolveBundleDeploymentID(deployArgs)
	if err != nil {
		return err
	}
	return BundleDuplicateFile(astroCoreClient, orgID, deployID, sourcePath, bundleDestination)
}

// --- bundle archive ---

func newBundleArchiveUploadCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "upload-archive [DEPLOYMENT-ID] <archive-file-path>",
		Aliases: []string{"up-archive"},
		Short:   "Upload a pre-built archive to a deployment bundle",
		Args:    cobra.RangeArgs(1, 2),
		RunE:    bundleArchiveUploadCmd,
	}
	cmd.Flags().StringVar(&bundleTargetPath, "target-path", "", "Remote target path within the bundle")
	cmd.Flags().BoolVar(&bundleNoOverwrite, "no-overwrite", false, "Do not overwrite existing files")
	return cmd
}

func bundleArchiveUploadCmd(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	var archivePath string
	var deployArgs []string
	if len(args) == 2 { //nolint:mnd
		deployArgs = args[:1]
		archivePath = args[1]
	} else {
		archivePath = args[0]
	}

	orgID, deployID, err := resolveBundleDeploymentID(deployArgs)
	if err != nil {
		return err
	}
	return BundleUploadArchive(bundleStreamClient, orgID, deployID, archivePath, bundleTargetPath, !bundleNoOverwrite)
}

func newBundleArchiveDownloadCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "download-archive [DEPLOYMENT-ID]",
		Aliases: []string{"dl-archive"},
		Short:   "Download a deployment bundle as an archive",
		Args:    cobra.MaximumNArgs(1),
		RunE:    bundleArchiveDownloadCmd(out),
	}
	cmd.Flags().StringVarP(&bundleArchiveOutputPath, "output", "o", "bundle.tar.gz", "Local output file path")
	return cmd
}

func bundleArchiveDownloadCmd(out io.Writer) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		orgID, deployID, err := resolveBundleDeploymentID(args)
		if err != nil {
			return err
		}
		return BundleDownloadArch(bundleStreamClient, orgID, deployID, bundleArchiveOutputPath, out)
	}
}
