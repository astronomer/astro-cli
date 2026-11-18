package deployment

import (
	"archive/tar"
	"cmp"
	"compress/gzip"
	httpContext "context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	astrov1alpha1 "github.com/astronomer/astro-cli/astro-client-v1alpha1"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/output"
)

const (
	bundleFileContentType = "application/octet-stream"
	bundleVersionHeader   = "x-astro-bundle-version"
	copiedFilePerm        = 0o644
	maxErrorBodyBytes     = 64 << 10
)

var (
	errBundleRequired     = errors.New("specify a bundle, as BUNDLE, BUNDLE:PATH or DEPLOYMENT_ID:BUNDLE:PATH")
	errDeploymentRequired = errors.New("specify a Deployment ID before the bundle, as DEPLOYMENT_ID:BUNDLE:PATH")
	errCopyNoBundle       = errors.New("one of SRC and DST must be a bundle path, as BUNDLE:PATH")
	errCopyStdinName      = errors.New("a copy from stdin needs a file name in DST")
	errMoveBetweenBundles = errors.New("moving from one bundle to another is not supported: copy with cp, then remove with rm")
	errCopyVersion        = errors.New("--version applies only when copying from a bundle")
	errCopyWriteFlags     = errors.New("--expected-version and --if-exists apply only when copying into a bundle")
	errCopyIfExists       = errors.New("--if-exists applies only to a copy with -r, or to a file copied within one bundle")
	errCopyStdin          = errors.New("-r cannot copy from stdin")
	errCopyStdout         = errors.New("-r cannot copy to stdout")
	errMoveNoBundle       = errors.New("both SRC and DST must be bundle paths, as BUNDLE:PATH")
	errMoveRoot           = errors.New("the bundle root cannot be moved")
	errMergeFile          = errors.New("--if-exists merge applies only to a directory")
	errCopyFilterFlags    = errors.New("--exclude, --include, --no-default-excludes and --no-gitignore apply only when copying a local directory into a bundle with -r")
)

// BundlePath is a path in a bundle, written BUNDLE:PATH, where BUNDLE is the
// bundle's name or ID and an empty PATH is the bundle root, or
// DEPLOYMENT_ID:BUNDLE:PATH for a bundle on a Deployment other than the one
// --deployment-id names.
type BundlePath struct {
	Deployment    string
	Bundle        string
	Path          string
	TrailingSlash bool
}

// isDir reports whether p names a directory: the bundle root, or a path written with a trailing slash.
func (p BundlePath) isDir() bool {
	return p.Path == "" || p.TrailingSlash
}

func (p BundlePath) String() string {
	prefix := p.Bundle + ":"
	if p.Deployment != "" {
		prefix = p.Deployment + ":" + prefix
	}
	switch {
	case p.Path == "":
		return prefix
	case p.TrailingSlash:
		return prefix + p.Path + "/"
	default:
		return prefix + p.Path
	}
}

// into returns the path name takes inside p when p names a directory, and p otherwise.
func (p BundlePath) into(name string) BundlePath {
	if !p.isDir() {
		return p
	}
	return BundlePath{Deployment: p.Deployment, Bundle: p.Bundle, Path: path.Join(p.Path, name)}
}

// ParseBundlePath parses BUNDLE, BUNDLE:PATH or DEPLOYMENT_ID:BUNDLE:PATH.
func ParseBundlePath(arg string) (BundlePath, error) {
	var p BundlePath
	bundle, filePath, _ := strings.Cut(arg, ":")
	afterDeployment, rest, found := strings.Cut(filePath, ":")
	if found {
		if bundle == "" {
			return BundlePath{}, errDeploymentRequired
		}
		p.Deployment, bundle, filePath = bundle, afterDeployment, rest
	}
	if bundle == "" {
		return BundlePath{}, errBundleRequired
	}
	p.Bundle = bundle
	p.Path = strings.Trim(filePath, "/")
	p.TrailingSlash = p.Path != "" && strings.HasSuffix(filePath, "/")
	return p, nil
}

// isLocalDir reports whether a local cp argument is written as a directory, with a trailing separator.
func isLocalDir(arg string) bool {
	return strings.HasSuffix(arg, "/") || strings.HasSuffix(arg, string(filepath.Separator))
}

// isBundlePath reports whether a cp argument is BUNDLE:PATH rather than a local
// path. On Windows, a single letter before the colon is a drive.
func isBundlePath(arg string) bool {
	bundle, _, found := strings.Cut(arg, ":")
	isDrive := runtime.GOOS == "windows" && len(bundle) == 1
	return found && bundle != "" && !isDrive && !strings.ContainsAny(bundle, `/\`)
}

// BundleCopyOptions are the flags of a copy to, from or within a bundle.
type BundleCopyOptions struct {
	Recursive       bool
	IfExists        string
	Version         string
	ExpectedVersion string
	Filter          BundleFilterOptions
}

// parseIfExists parses --if-exists, which is fail, replace or merge, or empty for
// the server's default of fail.
func parseIfExists(flag string) (string, error) {
	if flag == "" {
		return "", nil
	}
	ifExists := astrov1alpha1.MoveBundlePathRequestIfExists(strings.ToUpper(flag))
	if !ifExists.Valid() {
		return "", fmt.Errorf("invalid --if-exists %q: must be fail, replace or merge", flag)
	}
	return string(ifExists), nil
}

// bundleRef holds the IDs that the bundle file endpoints take.
type bundleRef struct {
	orgID, deploymentID, bundleID string
}

// bundleFileRow is one line of `bundle files ls` table output: a file, or a directory
// shown with a trailing slash.
type bundleFileRow struct {
	Path   string
	Size   string
	Digest string
}

func bundleFileTableConfig() *output.TableConfig {
	columns := []output.Column[bundleFileRow]{
		{Header: "PATH", Value: func(r bundleFileRow) string { return r.Path }},
		{Header: "SIZE", Value: func(r bundleFileRow) string { return r.Size }},
		{Header: "DIGEST", Value: func(r bundleFileRow) string { return r.Digest }},
	}
	return output.BuildTableConfig(
		columns,
		func(d any) []bundleFileRow {
			listing := d.(*astrov1alpha1.BundleFileList)
			rows := make([]bundleFileRow, 0, len(listing.Directories)+len(listing.Files))
			for _, dir := range listing.Directories {
				rows = append(rows, bundleFileRow{Path: path.Join(listing.Path, dir) + "/"})
			}
			for _, file := range listing.Files {
				rows = append(rows, bundleFileRow{Path: file.Path, Size: strconv.Itoa(file.Size), Digest: file.Digest})
			}
			return rows
		},
		output.WithNoResultsMsg("No files found in this bundle"),
	)
}

// ListBundleFilesData lists one directory of a bundle, or its whole subtree when
// recursive is set, at the bundle's current version or at an earlier one.
func ListBundleFilesData(target BundlePath, recursive bool, version, wsID, deploymentID string, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) (*astrov1alpha1.BundleFileList, error) {
	b, err := resolveBundle(target, wsID, deploymentID, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return nil, err
	}
	return listBundleFiles(target.Path, recursive, version, b, astroV1Alpha1Client)
}

func listBundleFiles(dir string, recursive bool, version string, b bundleRef, astroV1Alpha1Client astrov1alpha1.APIClient) (*astrov1alpha1.BundleFileList, error) {
	params := &astrov1alpha1.ListBundleFilesParams{}
	if dir != "" {
		params.Path = &dir
	}
	if recursive {
		params.Recursive = &recursive
	}
	if version != "" {
		params.Version = &version
	}

	resp, err := astroV1Alpha1Client.ListBundleFilesWithResponse(httpContext.Background(), b.orgID, b.deploymentID, b.bundleID, params)
	if err != nil {
		return nil, err
	}
	err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}

	return resp.JSON200, nil
}

// ListBundleFilesWithFormat prints a bundle's files in the requested format.
func ListBundleFilesWithFormat(target BundlePath, recursive bool, version, wsID, deploymentID string, format output.Format, tmpl string, out io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	return output.PrintData(
		func() (*astrov1alpha1.BundleFileList, error) {
			return ListBundleFilesData(target, recursive, version, wsID, deploymentID, astroV1Client, astroV1Alpha1Client)
		},
		bundleFileTableConfig(), format, tmpl, out,
	)
}

// CatBundleFile writes one file's content from a bundle to out, at the bundle's
// current version or at an earlier one.
func CatBundleFile(target BundlePath, version, wsID, deploymentID string, out io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) error {
	if target.isDir() {
		return fmt.Errorf("%s is a directory", target)
	}
	b, err := resolveBundle(target, wsID, deploymentID, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return err
	}
	content, _, err := openBundleFile(target.Path, version, b, astroV1Alpha1StreamClient)
	if err != nil {
		return err
	}
	defer content.Close()
	_, err = io.Copy(out, content)
	return err
}

// RemoveBundlePath removes one file from a bundle, or a directory and everything
// under it with recursive, where the bundle root empties the bundle.
// A recursive removal asks for confirmation unless force is set.
// expectedVersion, when set, makes the removal conditional on the bundle still
// being at that version.
func RemoveBundlePath(target BundlePath, recursive bool, expectedVersion, wsID, deploymentID string, force bool, out io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	if target.isDir() && !recursive {
		return fmt.Errorf("%s is a directory: remove it with -r", target)
	}
	b, err := resolveBundle(target, wsID, deploymentID, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return err
	}

	if recursive && !force {
		prompt := fmt.Sprintf("Are you sure you want to remove %s?", target)
		if target.Path == "" {
			prompt = fmt.Sprintf("Are you sure you want to remove every file in %s?", target.Bundle)
		}
		confirmed, _ := input.Confirm(prompt)
		if !confirmed {
			fmt.Fprintln(out, "Canceling removal")
			return nil
		}
	}

	var version string
	if recursive {
		params := &astrov1alpha1.DeleteBundleDirectoryParams{}
		if target.Path != "" {
			params.Path = &target.Path
		}
		if expectedVersion != "" {
			params.ExpectedVersion = &expectedVersion
		}
		resp, err := astroV1Alpha1Client.DeleteBundleDirectoryWithResponse(httpContext.Background(), b.orgID, b.deploymentID, b.bundleID, params)
		if err != nil {
			return err
		}
		err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return err
		}
		version = resp.JSON200.Version
	} else {
		params := &astrov1alpha1.DeleteBundleFileParams{Path: target.Path}
		if expectedVersion != "" {
			params.ExpectedVersion = &expectedVersion
		}
		resp, err := astroV1Alpha1Client.DeleteBundleFileWithResponse(httpContext.Background(), b.orgID, b.deploymentID, b.bundleID, params)
		if err != nil {
			return err
		}
		err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return err
		}
		version = resp.JSON200.Version
	}

	fmt.Fprintf(out, "Removed %s at version %s\n", target, version)
	return nil
}

// MoveBundlePath moves a file or a directory to another path in the same bundle.
func MoveBundlePath(src, dst, ifExists, expectedVersion, wsID, deploymentID string, out io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	ifExists, err := parseIfExists(ifExists)
	if err != nil {
		return err
	}
	if !isBundlePath(src) || !isBundlePath(dst) {
		return errMoveNoBundle
	}
	source, err := ParseBundlePath(src)
	if err != nil {
		return err
	}
	target, err := ParseBundlePath(dst)
	if err != nil {
		return err
	}
	if source.Path == "" {
		return errMoveRoot
	}
	target = target.into(path.Base(source.Path))

	b, dstBundle, err := resolveBundlePair(source, target, wsID, deploymentID, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return err
	}
	if dstBundle != b {
		return errMoveBetweenBundles
	}
	isDir, err := isBundleDirectory(source, expectedVersion, b, astroV1Alpha1Client)
	if err != nil {
		return err
	}
	if source.TrailingSlash && !isDir {
		return fmt.Errorf("%s is not a directory", source)
	}
	if !isDir && ifExists == string(astrov1alpha1.MoveBundlePathRequestIfExistsMERGE) {
		return errMergeFile
	}

	request := bundlePathRequest(source, target, ifExists, expectedVersion)
	var version string
	if isDir {
		resp, err := astroV1Alpha1Client.MoveBundleDirectoryWithResponse(httpContext.Background(), b.orgID, b.deploymentID, b.bundleID, request)
		if err != nil {
			return err
		}
		err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return err
		}
		version = resp.JSON200.Version
	} else {
		resp, err := astroV1Alpha1Client.MoveBundleFileWithResponse(httpContext.Background(), b.orgID, b.deploymentID, b.bundleID, request)
		if err != nil {
			return err
		}
		err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return err
		}
		version = resp.JSON200.Version
	}

	fmt.Fprintf(out, "Moved %s to %s at version %s\n", source, target, version)
	return nil
}

// CopyBundleFiles copies a file, or a directory's contents with Recursive, from a
// local path to a bundle path, from a bundle path to a local path, or between
// two bundle paths, on one Deployment or two. A local path of "-" is stdin or
// stdout.
func CopyBundleFiles(src, dst string, opts *BundleCopyOptions, wsID, deploymentID string, out io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) error {
	ifExists, err := parseIfExists(opts.IfExists)
	if err != nil {
		return err
	}
	opts.IfExists = ifExists

	srcInBundle, dstInBundle := isBundlePath(src), isBundlePath(dst)
	if opts.Filter.set() && (srcInBundle || !dstInBundle || !opts.Recursive) {
		return errCopyFilterFlags
	}
	switch {
	case srcInBundle && dstInBundle:
		source, err := ParseBundlePath(src)
		if err != nil {
			return err
		}
		target, err := ParseBundlePath(dst)
		if err != nil {
			return err
		}
		return copyBundleToBundle(source, target, opts, wsID, deploymentID, out, astroV1Client, astroV1Alpha1Client, astroV1Alpha1StreamClient)
	case dstInBundle:
		if opts.Version != "" {
			return errCopyVersion
		}
		target, err := ParseBundlePath(dst)
		if err != nil {
			return err
		}
		return copyToBundle(src, target, opts, wsID, deploymentID, out, astroV1Client, astroV1Alpha1Client, astroV1Alpha1StreamClient)
	case srcInBundle:
		if opts.ExpectedVersion != "" || opts.IfExists != "" {
			return errCopyWriteFlags
		}
		source, err := ParseBundlePath(src)
		if err != nil {
			return err
		}
		return copyFromBundle(source, dst, opts, wsID, deploymentID, out, astroV1Client, astroV1Alpha1Client, astroV1Alpha1StreamClient)
	default:
		return errCopyNoBundle
	}
}

func copyToBundle(src string, dst BundlePath, opts *BundleCopyOptions, wsID, deploymentID string, out io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) error {
	if src == "-" {
		if opts.Recursive {
			return errCopyStdin
		}
		if opts.IfExists != "" {
			return errCopyIfExists
		}
		if dst.isDir() {
			return errCopyStdinName
		}
		err := checkBundleFile("stdin", dst.Path, 0)
		if err != nil {
			return err
		}
		content, err := readBundleUpload(os.Stdin, "stdin")
		if err != nil {
			return err
		}
		return writeBundleFile(content, "stdin", dst, opts.ExpectedVersion, wsID, deploymentID, out, astroV1Client, astroV1Alpha1Client)
	}

	info, err := os.Stat(src)
	if err != nil {
		return errors.Wrapf(err, "failed to read %s", src)
	}
	if info.IsDir() {
		if !opts.Recursive {
			return fmt.Errorf("%s is a directory: copy it with -r", src)
		}
		return writeBundleDirectory(src, dst, opts, wsID, deploymentID, out, astroV1Client, astroV1Alpha1Client, astroV1Alpha1StreamClient)
	}
	if opts.IfExists != "" {
		return errCopyIfExists
	}
	target := dst.into(filepath.Base(src))
	err = checkBundleFile(src, target.Path, info.Size())
	if err != nil {
		return err
	}

	file, err := os.Open(src)
	if err != nil {
		return errors.Wrapf(err, "failed to read %s", src)
	}
	defer file.Close()
	// A pipe reports no size, and a file can grow after its size was checked.
	content, err := readBundleUpload(file, src)
	if err != nil {
		return err
	}
	return writeBundleFile(content, src, target, opts.ExpectedVersion, wsID, deploymentID, out, astroV1Client, astroV1Alpha1Client)
}

func copyFromBundle(src BundlePath, dst string, opts *BundleCopyOptions, wsID, deploymentID string, out io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) error {
	if opts.Recursive {
		if dst == "-" {
			return errCopyStdout
		}
		return readBundleDirectory(src, dst, opts.Version, wsID, deploymentID, out, astroV1Client, astroV1Alpha1Client, astroV1Alpha1StreamClient)
	}
	if src.isDir() {
		return fmt.Errorf("%s is a directory: copy it with -r", src)
	}

	b, err := resolveBundle(src, wsID, deploymentID, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return err
	}
	content, version, err := openBundleFile(src.Path, opts.Version, b, astroV1Alpha1StreamClient)
	if err != nil {
		return err
	}
	defer content.Close()
	if dst == "-" {
		_, err = io.Copy(out, content)
		return err
	}

	if isLocalDir(dst) {
		err = os.MkdirAll(dst, os.ModePerm)
		if err != nil {
			return err
		}
	}
	info, err := os.Stat(dst)
	if err == nil && info.IsDir() {
		dst = filepath.Join(dst, path.Base(src.Path))
	}
	root, err := os.OpenRoot(filepath.Dir(dst))
	if err != nil {
		return err
	}
	defer root.Close()
	err = writeLocalFile(root, filepath.Base(dst), content)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Copied %s at version %s to %s\n", src, version, dst)
	return nil
}

// copyBundleToBundle copies within one bundle on the server, or else passes
// what it reads from SRC to the write to DST: a directory streams, and a file,
// at most 1 MiB, is read whole.
func copyBundleToBundle(src, dst BundlePath, opts *BundleCopyOptions, wsID, deploymentID string, out io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) error {
	if !opts.Recursive {
		if src.isDir() {
			return fmt.Errorf("%s is a directory: copy it with -r", src)
		}
		if opts.IfExists == string(astrov1alpha1.MoveBundlePathRequestIfExistsMERGE) {
			return errMergeFile
		}
		dst = dst.into(path.Base(src.Path))
	}
	source, target, err := resolveBundlePair(src, dst, wsID, deploymentID, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return err
	}
	if source == target && opts.Version == "" {
		return copyWithinBundle(src, dst, source, opts, out, astroV1Alpha1Client)
	}
	if !opts.Recursive && opts.IfExists != "" {
		return errCopyIfExists
	}

	var readVersion, version string
	if opts.Recursive {
		next, archiveVersion, err := openBundleDirectory(src.Path, opts.Version, source, astroV1Alpha1StreamClient)
		if err != nil {
			return err
		}
		readVersion = archiveVersion
		// A write the rate limits refuse is sent again, reading SRC again at the version read first.
		pack := func(w io.Writer) error {
			archive := next
			next = nil
			if archive == nil {
				var err error
				archive, _, err = openBundleDirectory(src.Path, cmp.Or(readVersion, opts.Version), source, astroV1Alpha1StreamClient)
				if err != nil {
					return err
				}
			}
			defer archive.Close()
			_, err := io.Copy(w, archive)
			return err
		}
		version, err = putBundleDirectory(pack, bundleDirectoryParams(dst.Path, opts.IfExists, opts.ExpectedVersion), target, astroV1Alpha1StreamClient)
		if err != nil {
			return err
		}
	} else {
		content, contentVersion, err := openBundleFile(src.Path, opts.Version, source, astroV1Alpha1StreamClient)
		if err != nil {
			return err
		}
		defer content.Close()
		readVersion = contentVersion
		version, err = putBundleFile(content, dst.Path, opts.ExpectedVersion, target, astroV1Alpha1Client)
		if err != nil {
			return err
		}
	}

	fmt.Fprintf(out, "Copied %s at version %s to %s at version %s\n", src, readVersion, dst, version)
	return nil
}

func copyWithinBundle(src, dst BundlePath, b bundleRef, opts *BundleCopyOptions, out io.Writer, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	request := bundlePathRequest(src, dst, opts.IfExists, opts.ExpectedVersion)
	var version string
	if opts.Recursive {
		resp, err := astroV1Alpha1Client.DuplicateBundleDirectoryWithResponse(httpContext.Background(), b.orgID, b.deploymentID, b.bundleID, request)
		if err != nil {
			return err
		}
		err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return err
		}
		version = resp.JSON200.Version
	} else {
		resp, err := astroV1Alpha1Client.DuplicateBundleFileWithResponse(httpContext.Background(), b.orgID, b.deploymentID, b.bundleID, request)
		if err != nil {
			return err
		}
		err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return err
		}
		version = resp.JSON200.Version
	}

	fmt.Fprintf(out, "Copied %s to %s at version %s\n", src, dst, version)
	return nil
}

// bundlePathRequest is the request body of a move or a copy within a bundle.
func bundlePathRequest(src, dst BundlePath, ifExists, expectedVersion string) astrov1alpha1.MoveBundlePathRequest {
	request := astrov1alpha1.MoveBundlePathRequest{FromPath: src.Path, ToPath: &dst.Path}
	if ifExists != "" {
		request.IfExists = new(astrov1alpha1.MoveBundlePathRequestIfExists(ifExists))
	}
	if expectedVersion != "" {
		request.ExpectedVersion = &expectedVersion
	}
	return request
}

// isBundleDirectory reports whether target is a directory rather than a file, at
// version when set or else at the bundle's current version.
func isBundleDirectory(target BundlePath, version string, b bundleRef, astroV1Alpha1Client astrov1alpha1.APIClient) (bool, error) {
	parent := path.Dir(target.Path)
	if parent == "." {
		parent = ""
	}
	listing, err := listBundleFiles(parent, false, version, b, astroV1Alpha1Client)
	if err != nil {
		return false, err
	}

	if slices.Contains(listing.Directories, path.Base(target.Path)) {
		return true, nil
	}
	for _, file := range listing.Files {
		if file.Path == target.Path {
			return false, nil
		}
	}
	return false, fmt.Errorf("no file or directory at %s", target)
}

// openBundleFile returns a stream of one file's content, which the caller
// closes, and the bundle version it was read from.
func openBundleFile(filePath, version string, b bundleRef, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) (content io.ReadCloser, readVersion string, err error) {
	params := &astrov1alpha1.ReadBundleFileParams{Path: filePath}
	if version != "" {
		params.Version = &version
	}

	ctx := httpContext.Background()
	resp, err := httputil.RetryRateLimited(ctx, func() (*http.Response, error) {
		return astroV1Alpha1StreamClient.ReadBundleFile(ctx, b.orgID, b.deploymentID, b.bundleID, params)
	})
	if err != nil {
		return nil, "", err
	}
	err = streamedResponseError(resp)
	if err != nil {
		return nil, "", err
	}

	return resp.Body, resp.Header.Get(bundleVersionHeader), nil
}

// openBundleDirectory returns a stream of dir as a gzipped tar, which the caller
// closes, and the bundle version it was read from.
func openBundleDirectory(dir, version string, b bundleRef, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) (archive io.ReadCloser, readVersion string, err error) {
	params := &astrov1alpha1.ReadBundleDirectoryParams{}
	if dir != "" {
		params.Path = &dir
	}
	if version != "" {
		params.Version = &version
	}

	ctx := httpContext.Background()
	resp, err := httputil.RetryRateLimited(ctx, func() (*http.Response, error) {
		return astroV1Alpha1StreamClient.ReadBundleDirectory(ctx, b.orgID, b.deploymentID, b.bundleID, params)
	})
	if err != nil {
		return nil, "", err
	}
	err = streamedResponseError(resp)
	if err != nil {
		return nil, "", err
	}

	return resp.Body, resp.Header.Get(bundleVersionHeader), nil
}

// streamedResponseError returns the API error of a response whose body is left
// unread on success, closing the body when there is an error.
func streamedResponseError(resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	if err != nil {
		return err
	}
	return astrov1alpha1.NormalizeAPIError(resp, body)
}

func writeBundleFile(content io.Reader, src string, dst BundlePath, expectedVersion, wsID, deploymentID string, out io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	b, err := resolveBundle(dst, wsID, deploymentID, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return err
	}
	version, err := putBundleFile(content, dst.Path, expectedVersion, b, astroV1Alpha1Client)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Copied %s to %s at version %s\n", src, dst, version)
	return nil
}

// putBundleFile writes content to filePath in a bundle and returns the version the write created.
func putBundleFile(content io.Reader, filePath, expectedVersion string, b bundleRef, astroV1Alpha1Client astrov1alpha1.APIClient) (string, error) {
	params := &astrov1alpha1.WriteBundleFileParams{Path: filePath}
	if expectedVersion != "" {
		params.ExpectedVersion = &expectedVersion
	}

	resp, err := astroV1Alpha1Client.WriteBundleFileWithBodyWithResponse(httpContext.Background(), b.orgID, b.deploymentID, b.bundleID, params, bundleFileContentType, content)
	if err != nil {
		return "", err
	}
	err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return "", err
	}
	return resp.JSON200.Version, nil
}

func readBundleDirectory(src BundlePath, dst, version, wsID, deploymentID string, out io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) error {
	b, err := resolveBundle(src, wsID, deploymentID, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return err
	}
	archive, readVersion, err := openBundleDirectory(src.Path, version, b, astroV1Alpha1StreamClient)
	if err != nil {
		return err
	}
	defer archive.Close()
	files, err := unpackBundleDirectory(archive, dst, nil)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Copied %d files from %s at version %s to %s\n", files, src, readVersion, dst)
	return nil
}

func writeBundleDirectory(src string, dst BundlePath, opts *BundleCopyOptions, wsID, deploymentID string, out io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) error {
	src, err := followLocalDir(src)
	if err != nil {
		return err
	}
	filter, err := newLocalFileFilter(src, opts.Filter)
	if err != nil {
		return err
	}
	files, skipped, err := localFiles(src, filter)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		if skipped > 0 {
			return fmt.Errorf("%s holds no files to copy once %d excluded paths are skipped", src, skipped)
		}
		return fmt.Errorf("%s holds no files to copy", src)
	}
	err = checkBundleUpload(src, dst.Path, files)
	if err != nil {
		return err
	}

	b, err := resolveBundle(dst, wsID, deploymentID, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return err
	}
	version, err := uploadBundleDirectory(src, files, bundleDirectoryParams(dst.Path, opts.IfExists, opts.ExpectedVersion), b, astroV1Alpha1StreamClient)
	if err != nil {
		return err
	}

	printSkipped(out, skipped)
	fmt.Fprintf(out, "Copied %d files from %s to %s at version %s\n", len(files), src, dst, version)
	return nil
}

// bundleDirectoryParams are the parameters of a directory write to dir.
func bundleDirectoryParams(dir, ifExists, expectedVersion string) *astrov1alpha1.WriteBundleDirectoryParams {
	params := &astrov1alpha1.WriteBundleDirectoryParams{}
	if dir != "" {
		params.Path = &dir
	}
	if ifExists != "" {
		params.IfExists = new(astrov1alpha1.WriteBundleDirectoryParamsIfExists(ifExists))
	}
	if expectedVersion != "" {
		params.ExpectedVersion = &expectedVersion
	}
	return params
}

// putBundleDirectory writes a gzipped tar to a bundle directory and returns the
// version the write created. pack writes the tar while the request sends it,
// and writes it again each time the rate limits refuse the request.
func putBundleDirectory(pack func(io.Writer) error, params *astrov1alpha1.WriteBundleDirectoryParams, b bundleRef, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) (string, error) {
	ctx := httpContext.Background()
	var packErr error
	resp, err := httputil.RetryRateLimited(ctx, func() (*http.Response, error) {
		body := newPipedBody(pack)
		resp, err := astroV1Alpha1StreamClient.WriteBundleDirectoryWithBody(ctx, b.orgID, b.deploymentID, b.bundleID, params, bundleFileContentType, body)
		packErr = body.Close()
		if packErr == nil {
			return resp, err
		}
		if err == nil {
			resp.Body.Close()
		}
		return nil, packErr
	})
	if packErr != nil {
		return "", packErr
	}
	if err != nil {
		return "", err
	}
	parsed, err := astrov1alpha1.ParseWriteBundleDirectoryResponse(resp)
	if err != nil {
		return "", err
	}
	err = astrov1alpha1.NormalizeAPIError(parsed.HTTPResponse, parsed.Body)
	if err != nil {
		return "", err
	}
	return parsed.JSON200.Version, nil
}

// pipedBody is a request body that a function writes as the request reads it.
type pipedBody struct {
	*io.PipeReader
	// wait waits for the write to end and returns the error it failed with.
	wait func() error
}

func newPipedBody(write func(io.Writer) error) *pipedBody {
	reader, writer := io.Pipe()
	done := make(chan struct{})
	var writeErr error
	go func() {
		defer close(done)
		err := write(writer)
		writer.CloseWithError(err)
		// A closed pipe means the request stopped reading, and its own error says why.
		if !errors.Is(err, io.ErrClosedPipe) {
			writeErr = err
		}
	}()
	wait := func() error {
		<-done
		return writeErr
	}
	return &pipedBody{PipeReader: reader, wait: wait}
}

// Close stops the write, if the request did not read all of it, and returns the error the write failed with.
func (b *pipedBody) Close() error {
	b.PipeReader.Close()
	return b.wait()
}

// uploadBundleDirectory writes files, by slash-separated paths relative to
// localDir, to a bundle directory as a gzipped tar, and returns the version the
// write created.
func uploadBundleDirectory(localDir string, files []string, params *astrov1alpha1.WriteBundleDirectoryParams, b bundleRef, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) (string, error) {
	pack := func(w io.Writer) error {
		return packBundleDirectory(w, localDir, files)
	}
	return putBundleDirectory(pack, params, b, astroV1Alpha1StreamClient)
}

// resolveDeployment returns the Deployment whose ID a bundle path holds, else
// the one deploymentID names, else the one the user selects.
func resolveDeployment(pathDeploymentID, wsID, deploymentID string, astroV1Client astrov1.APIClient) (astrov1.Deployment, error) {
	if pathDeploymentID != "" {
		return GetDeploymentByID("", pathDeploymentID, astroV1Client)
	}
	return GetDeployment(wsID, deploymentID, "", false, nil, astroV1Client)
}

// resolveBundle resolves the bundle of target, named by its ID or its Dag bundle name.
func resolveBundle(target BundlePath, wsID, deploymentID string, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) (bundleRef, error) {
	dep, err := resolveDeployment(target.Deployment, wsID, deploymentID, astroV1Client)
	if err != nil {
		return bundleRef{}, err
	}
	bundles, err := listAllBundles(dep.OrganizationId, dep.Id, astroV1Alpha1Client)
	if err != nil {
		return bundleRef{}, err
	}
	bundleID, err := findBundleID(bundles, target.Bundle, dep.Id)
	if err != nil {
		return bundleRef{}, err
	}
	return bundleRef{orgID: dep.OrganizationId, deploymentID: dep.Id, bundleID: bundleID}, nil
}

// resolveBundlePair resolves the bundles of src and dst, looking up a Deployment they share once.
func resolveBundlePair(src, dst BundlePath, wsID, deploymentID string, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) (source, target bundleRef, err error) {
	srcDep, err := resolveDeployment(src.Deployment, wsID, deploymentID, astroV1Client)
	if err != nil {
		return bundleRef{}, bundleRef{}, err
	}
	srcBundles, err := listAllBundles(srcDep.OrganizationId, srcDep.Id, astroV1Alpha1Client)
	if err != nil {
		return bundleRef{}, bundleRef{}, err
	}
	dstDep, dstBundles := srcDep, srcBundles
	if dst.Deployment != src.Deployment {
		dstDep, err = resolveDeployment(dst.Deployment, wsID, deploymentID, astroV1Client)
		if err != nil {
			return bundleRef{}, bundleRef{}, err
		}
		if dstDep.Id != srcDep.Id {
			dstBundles, err = listAllBundles(dstDep.OrganizationId, dstDep.Id, astroV1Alpha1Client)
			if err != nil {
				return bundleRef{}, bundleRef{}, err
			}
		}
	}

	srcBundleID, err := findBundleID(srcBundles, src.Bundle, srcDep.Id)
	if err != nil {
		return bundleRef{}, bundleRef{}, err
	}
	dstBundleID, err := findBundleID(dstBundles, dst.Bundle, dstDep.Id)
	if err != nil {
		return bundleRef{}, bundleRef{}, err
	}
	source = bundleRef{orgID: srcDep.OrganizationId, deploymentID: srcDep.Id, bundleID: srcBundleID}
	target = bundleRef{orgID: dstDep.OrganizationId, deploymentID: dstDep.Id, bundleID: dstBundleID}
	return source, target, nil
}

// findBundleID returns the ID of the bundle whose ID or, failing that, whose name is ref.
func findBundleID(bundles []astrov1alpha1.DeploymentBundle, ref, deploymentID string) (string, error) {
	for i := range bundles {
		if bundles[i].Id == ref {
			return ref, nil
		}
	}
	for i := range bundles {
		if bundles[i].Name != nil && *bundles[i].Name == ref {
			return bundles[i].Id, nil
		}
	}
	return "", fmt.Errorf("no bundle with the ID or name %q on deployment %s", ref, deploymentID)
}

// packBundleDirectory writes to archive a gzipped tar of files, by slash-separated
// paths relative to localDir.
func packBundleDirectory(archive io.Writer, localDir string, files []string) error {
	gzipWriter := gzip.NewWriter(archive)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, relPath := range files {
		file, err := os.Open(filepath.Join(localDir, filepath.FromSlash(relPath)))
		if err != nil {
			return errors.Wrapf(err, "failed to read %s", relPath)
		}
		info, err := file.Stat()
		if err == nil {
			err = tarWriter.WriteHeader(&tar.Header{Name: relPath, Mode: 0o644, Size: info.Size(), Typeflag: tar.TypeReg})
		}
		if err == nil {
			_, err = io.Copy(tarWriter, file)
		}
		closeErr := file.Close()
		if err != nil {
			return errors.Wrapf(err, "failed to read %s", relPath)
		}
		if closeErr != nil {
			return closeErr
		}
	}
	err := tarWriter.Close()
	if err != nil {
		return err
	}
	return gzipWriter.Close()
}

// unpackBundleDirectory writes the regular files in a gzipped tar under
// localDir, or only those keep accepts when it is set, refusing any path that
// would land outside localDir or on a file it already wrote, and returns how
// many files it wrote.
func unpackBundleDirectory(archive io.Reader, localDir string, keep func(relPath string) bool) (int, error) {
	err := os.MkdirAll(localDir, os.ModePerm)
	if err != nil {
		return 0, err
	}
	root, err := os.OpenRoot(localDir)
	if err != nil {
		return 0, err
	}
	defer root.Close()

	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read the bundle directory")
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	// Each file written, by its path in lower case, to catch two that a case-insensitive file system would make one.
	written := map[string]string{}
	files := 0
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			// Reading to the end of the gzip stream checks its checksum, which a stream cut short between two entries fails.
			padding, err := io.Copy(io.Discard, io.LimitReader(gzipReader, maxArchivePaddingBytes+1))
			if err == nil && padding > maxArchivePaddingBytes {
				err = fmt.Errorf("it holds more than %d bytes past the end of its tar", maxArchivePaddingBytes)
			}
			if err != nil {
				return files, errors.Wrap(err, "failed to read the bundle directory")
			}
			return files, nil
		}
		if err != nil {
			return files, errors.Wrap(err, "failed to read the bundle directory")
		}
		if header.Typeflag != tar.TypeReg || (keep != nil && !keep(header.Name)) {
			continue
		}
		name := filepath.FromSlash(header.Name)
		lower := strings.ToLower(header.Name)
		earlier, found := written[lower]
		if found && sameLocalFile(root, filepath.FromSlash(earlier), name) {
			return files, fmt.Errorf("%s and %s in the bundle are one file in %s, whose file system ignores case", earlier, header.Name, localDir)
		}
		err = root.MkdirAll(filepath.Dir(name), os.ModePerm)
		if err != nil {
			return files, err
		}
		err = writeLocalFile(root, name, tarReader)
		if err != nil {
			return files, err
		}
		written[lower] = header.Name
		files++
	}
}

// sameLocalFile reports whether a and b under root name one existing file.
func sameLocalFile(root *os.Root, a, b string) bool {
	aInfo, err := root.Stat(a)
	if err != nil {
		return false
	}
	bInfo, err := root.Stat(b)
	return err == nil && os.SameFile(aInfo, bInfo)
}

// writeLocalFile writes content to name under root through a temporary file
// beside it, so a failed write leaves any file already at name as it was.
func writeLocalFile(root *os.Root, name string, content io.Reader) error {
	perm := os.FileMode(copiedFilePerm)
	info, err := root.Stat(name)
	if err == nil && info.Mode().IsRegular() {
		perm = info.Mode().Perm()
	}
	temp := filepath.Join(filepath.Dir(name), ".astro-"+rand.Text())
	file, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	_, err = io.Copy(file, content) //nolint:gosec // the server bounds a bundle's size
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = root.Rename(temp, name)
	}
	if err != nil {
		_ = root.Remove(temp)
		return err
	}
	return nil
}
