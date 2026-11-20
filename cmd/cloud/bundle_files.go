package cloud

import (
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/pkg/output"
)

var (
	filesRecursive       bool
	filesForce           bool
	filesNoDelete        bool
	filesDryRun          bool
	filesFilter          deployment.BundleFilterOptions
	filesIfExists        string
	filesVersion         string
	filesExpectedVersion string
	filesListOutputFlags output.Flags
)

func newBundleFilesRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "files",
		Aliases: []string{"file"},
		Hidden:  true,
		Short:   "Manage the files in file bundles",
		Long:    "List, read, copy, sync, move and remove the files in file bundles. A path in a bundle is written BUNDLE:PATH, where BUNDLE is the bundle's name or ID, and a trailing slash names a directory. DEPLOYMENT_ID:BUNDLE:PATH names a bundle on another Deployment than --deployment-id. Every write creates a new version of the bundle.\n\nA bundle holds at most 10,000 files of at most 1 MiB each and 100 MiB in all, at paths of at most 192 bytes made of ASCII letters, digits, '/', '.', '_' and '-'. A copy or sync from local files checks them first, and names every file that breaks these limits before sending anything.",
	}
	cmd.AddCommand(
		newBundleFilesListCmd(out),
		newBundleFilesCatCmd(out),
		newBundleFilesCopyCmd(out),
		newBundleFilesSyncCmd(out),
		newBundleFilesMoveCmd(out),
		newBundleFilesRemoveCmd(out),
	)
	return cmd
}

func newBundleFilesListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ls BUNDLE[:PATH]",
		Aliases: []string{"list"},
		Short:   "List the files in a file bundle",
		Long:    "List the files and directories in one directory of a file bundle, or every file under it with -r. Omit PATH to list the bundle root.",
		Example: `  # List the bundle root
  astro bundle files ls my-files --deployment-id <id>

  # List every file under a directory
  astro bundle files ls my-files:dags -r --deployment-id <id>

  # List an earlier version of the bundle
  astro bundle files ls my-files --version <version> --deployment-id <id> --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := filesListOutputFlags.Resolve()
			if err != nil {
				return err
			}
			target, err := deployment.ParseBundlePath(args[0])
			if err != nil {
				return err
			}
			ws, err := coalesceWorkspace()
			if err != nil {
				return errors.Wrap(err, "failed to find a valid workspace")
			}
			cmd.SilenceUsage = true
			return deployment.ListBundleFilesWithFormat(target, filesRecursive, filesVersion, ws, deploymentID, format, filesListOutputFlags.Template, out, astroV1Client, astroV1Alpha1Client)
		},
	}
	cmd.Flags().BoolVarP(&filesRecursive, "recursive", "r", false, "List every file under PATH instead of one directory level")
	cmd.Flags().StringVar(&filesVersion, "version", "", "List an earlier version of the bundle instead of its current one")
	filesListOutputFlags.AddFlags(cmd)
	return cmd
}

func newBundleFilesCatCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cat BUNDLE:PATH",
		Short: "Print a file from a file bundle",
		Long:  "Print the content of one file in a file bundle to stdout.",
		Example: `  astro bundle files cat my-files:dags/etl.py --deployment-id <id>
  astro bundle files cat my-files:dags/etl.py --version <version> --deployment-id <id> > etl_old.py`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := deployment.ParseBundlePath(args[0])
			if err != nil {
				return err
			}
			ws, err := coalesceWorkspace()
			if err != nil {
				return errors.Wrap(err, "failed to find a valid workspace")
			}
			cmd.SilenceUsage = true
			return deployment.CatBundleFile(target, filesVersion, ws, deploymentID, out, astroV1Client, astroV1Alpha1Client, astroV1Alpha1StreamClient)
		},
	}
	cmd.Flags().StringVar(&filesVersion, "version", "", "Read the file from an earlier version of the bundle instead of its current one")
	return cmd
}

func newBundleFilesCopyCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cp SRC DST",
		Aliases: []string{"copy"},
		Short:   "Copy files to, from, within or between file bundles",
		Long: `Copy a file, or a directory's contents with -r, between a local path and a file bundle, or between two bundle paths, in one bundle or in two, on one Deployment or two. A bundle path is written BUNDLE:PATH, and the whole bundle BUNDLE:, prefixed with DEPLOYMENT_ID: for a bundle on another Deployment; any other path is local, and '-' is stdin or stdout. A file copied to a DST ending in '/' keeps its name inside that directory. With -r, the contents of SRC go into DST. A download replaces a local file only once it has arrived whole.

A copy of a directory, or of a file within one bundle, fails if something is already at DST, unless --if-exists says to replace it or merge into it. Any other file copied into a bundle always replaces the file at DST.

A local directory copied into a bundle skips version control, OS, Python, Node and editor files and .env files, and whatever .gitignore files ignore. --include, --exclude, --no-default-excludes and --no-gitignore change that. A symbolic link under it that is not skipped fails the copy.

A copy into a bundle creates one new bundle version and prints it.`,
		Example: `  # Copy one file into the bundle, and back out
  astro bundle files cp ./etl.py my-files:dags/ --deployment-id <id>
  astro bundle files cp my-files:dags/etl.py ./ --deployment-id <id>

  # Copy from stdin
  generate_dag | astro bundle files cp - my-files:dags/generated.py --deployment-id <id>

  # Copy a directory's contents into the bundle, replacing what was there
  astro bundle files cp -r ./dags my-files:dags --if-exists replace --deployment-id <id>

  # Copy a directory within the bundle
  astro bundle files cp -r my-files:dags my-files:dags_backup --deployment-id <id>

  # Copy a directory to a bundle on another Deployment
  astro bundle files cp -r my-files:dags <other-id>:my-files:dags --deployment-id <id>

  # Copy the whole bundle, as it was at an earlier version, to a local directory
  astro bundle files cp -r my-files: ./my-files --version <version> --deployment-id <id>

  # Copy only if nobody has written to the bundle since the version you last read
  astro bundle files cp ./etl.py my-files:dags/etl.py --expected-version <version> --deployment-id <id>`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := coalesceWorkspace()
			if err != nil {
				return errors.Wrap(err, "failed to find a valid workspace")
			}
			cmd.SilenceUsage = true
			opts := &deployment.BundleCopyOptions{
				Recursive:       filesRecursive,
				IfExists:        filesIfExists,
				Version:         filesVersion,
				ExpectedVersion: filesExpectedVersion,
				Filter:          filesFilter,
			}
			return deployment.CopyBundleFiles(args[0], args[1], opts, ws, deploymentID, out, astroV1Client, astroV1Alpha1Client, astroV1Alpha1StreamClient)
		},
	}
	cmd.Flags().BoolVarP(&filesRecursive, "recursive", "r", false, "Copy a directory's contents")
	cmd.Flags().StringVar(&filesIfExists, "if-exists", "", "What to do when something is already at DST in the bundle: fail, the default; replace it; or, for a directory, merge into it")
	cmd.Flags().StringVar(&filesVersion, "version", "", "Copy from an earlier version of the SRC bundle instead of its current one")
	cmd.Flags().StringVar(&filesExpectedVersion, "expected-version", "", "Copy into the DST bundle only if it is still at this version")
	addBundleFilesFilterFlags(cmd)
	return cmd
}

func newBundleFilesSyncCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync SRC DST",
		Short: "Make a local directory and a file bundle directory match",
		Long: `Make DST match SRC, where one is a local directory and the other a bundle directory, written BUNDLE:PATH or BUNDLE: for the whole bundle. Only the files whose content differs are transferred, and the files only DST holds are deleted unless --no-delete is set. The changes are printed, and --dry-run prints them without making them.

A sync into a bundle creates one new bundle version, and fails if the bundle changes between the sync reading it and writing it. A sync from a bundle asks before it deletes local files, unless -f, and fails if the bundle path holds no files.

A sync skips version control, OS, Python, Node and editor files and .env files, and whatever .gitignore files ignore, and never deletes them. --include, --exclude, --no-default-excludes and --no-gitignore change that. A symbolic link under the local directory that is not skipped fails the sync.`,
		Example: `  # Upload what changed, deleting the files you removed locally
  astro bundle files sync ./dags my-files:dags --deployment-id <id>

  # Upload what changed, keeping the files only the bundle holds
  astro bundle files sync ./dags my-files:dags --no-delete --deployment-id <id>

  # Check what a sync would change, then apply exactly that
  astro bundle files sync ./dags my-files:dags --dry-run --deployment-id <id>
  astro bundle files sync ./dags my-files:dags --expected-version <version> --deployment-id <id>

  # Download the whole bundle as it was at an earlier version
  astro bundle files sync my-files: ./my-files --version <version> --deployment-id <id>`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := coalesceWorkspace()
			if err != nil {
				return errors.Wrap(err, "failed to find a valid workspace")
			}
			cmd.SilenceUsage = true
			opts := &deployment.BundleSyncOptions{
				NoDelete:        filesNoDelete,
				Force:           filesForce,
				DryRun:          filesDryRun,
				Version:         filesVersion,
				ExpectedVersion: filesExpectedVersion,
				Filter:          filesFilter,
			}
			return deployment.SyncBundleFiles(cmd.Context(), args[0], args[1], opts, ws, deploymentID, out, astroV1Client, astroV1Alpha1Client, astroV1Alpha1StreamClient)
		},
	}
	cmd.Flags().BoolVar(&filesNoDelete, "no-delete", false, "Keep the files only DST holds instead of deleting them")
	cmd.Flags().BoolVarP(&filesForce, "force", "f", false, "Delete local files without confirmation, in a sync from a bundle")
	cmd.Flags().BoolVar(&filesDryRun, "dry-run", false, "Print the changes without making them")
	cmd.Flags().StringVar(&filesVersion, "version", "", "Sync from an earlier version of the bundle instead of its current one")
	cmd.Flags().StringVar(&filesExpectedVersion, "expected-version", "", "Sync into the bundle only if it is still at this version")
	addBundleFilesFilterFlags(cmd)
	return cmd
}

func addBundleFilesFilterFlags(cmd *cobra.Command) {
	cmd.Flags().StringArrayVar(&filesFilter.Excludes, "exclude", nil, "Skip local files matching this gitignore pattern. Repeatable")
	cmd.Flags().StringArrayVar(&filesFilter.Includes, "include", nil, "Keep local files matching this gitignore pattern, even when they would be skipped. Inside a skipped directory, the pattern must name a path through it, such as node_modules/pkg/. Repeatable")
	cmd.Flags().BoolVar(&filesFilter.NoDefaultExcludes, "no-default-excludes", false, "Keep version control, OS, Python, Node and editor files and .env files")
	cmd.Flags().BoolVar(&filesFilter.NoGitignore, "no-gitignore", false, "Keep files that .gitignore files ignore")
}

func newBundleFilesMoveCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "mv SRC DST",
		Aliases: []string{"move"},
		Short:   "Move a file or a directory within a file bundle",
		Long:    "Move a file or a directory to another path in the same file bundle, written BUNDLE:PATH, and print the bundle version the move created. SRC keeps its name inside a DST ending in '/'. The move fails if something is already at DST, unless --if-exists says to replace it or merge into it.",
		Example: `  astro bundle files mv my-files:dags/etl.py my-files:dags/legacy/ --deployment-id <id>
  astro bundle files mv my-files:staging my-files:dags --if-exists merge --deployment-id <id>`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := coalesceWorkspace()
			if err != nil {
				return errors.Wrap(err, "failed to find a valid workspace")
			}
			cmd.SilenceUsage = true
			return deployment.MoveBundlePath(args[0], args[1], filesIfExists, filesExpectedVersion, ws, deploymentID, out, astroV1Client, astroV1Alpha1Client)
		},
	}
	cmd.Flags().StringVar(&filesIfExists, "if-exists", "", "What to do when something is already at DST: fail, the default; replace it; or, for a directory, merge into it")
	cmd.Flags().StringVar(&filesExpectedVersion, "expected-version", "", "Move only if the bundle is still at this version")
	return cmd
}

func newBundleFilesRemoveCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rm BUNDLE:PATH",
		Aliases: []string{"delete"},
		Short:   "Remove a file or a directory from a file bundle",
		Long:    "Remove one file from a file bundle, or a directory and everything under it with -r, and print the bundle version the removal created. With -r, the bundle root removes every file in the bundle, and the removal asks for confirmation unless -f. 'astro bundle rollback' undoes a removal.",
		Example: `  astro bundle files rm my-files:dags/etl.py --deployment-id <id>
  astro bundle files rm my-files:dags/etl.py --expected-version <version> --deployment-id <id>

  # Remove a directory, or every file in the bundle
  astro bundle files rm -r my-files:dags/legacy --deployment-id <id>
  astro bundle files rm -r -f my-files: --deployment-id <id>`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !strings.Contains(args[0], ":") {
				return fmt.Errorf("%s names no path in a bundle: write BUNDLE:PATH, or BUNDLE: for every file in the bundle", args[0])
			}
			target, err := deployment.ParseBundlePath(args[0])
			if err != nil {
				return err
			}
			ws, err := coalesceWorkspace()
			if err != nil {
				return errors.Wrap(err, "failed to find a valid workspace")
			}
			cmd.SilenceUsage = true
			return deployment.RemoveBundlePath(target, filesRecursive, filesExpectedVersion, ws, deploymentID, filesForce, out, astroV1Client, astroV1Alpha1Client)
		},
	}
	cmd.Flags().BoolVarP(&filesRecursive, "recursive", "r", false, "Remove a directory and everything under it")
	cmd.Flags().StringVar(&filesExpectedVersion, "expected-version", "", "Remove only if the bundle is still at this version")
	cmd.Flags().BoolVarP(&filesForce, "force", "f", false, "Remove a directory without confirmation")
	return cmd
}
