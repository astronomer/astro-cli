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

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	"github.com/astronomer/astro-cli/internal/platform/astro/pagination"
	"github.com/astronomer/astro-cli/internal/usageerr"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/output"
	"github.com/astronomer/astro-cli/pkg/util"
)

const (
	bundleFileContentType = "application/octet-stream"
	bundleVersionHeader   = "x-astro-bundle-version"
	copiedFilePerm        = 0o644
	maxErrorBodyBytes     = 64 << 10
)

var (
	errNoFileList         = errors.New("the API returned no file list")
	errBundleRequired     = usageerr.Usage(errors.New("specify a bundle, as BUNDLE, BUNDLE:PATH or DEPLOYMENT_ID:BUNDLE:PATH"))
	errDeploymentRequired = usageerr.Usage(errors.New("specify a Deployment ID before the bundle, as DEPLOYMENT_ID:BUNDLE:PATH"))
	errCopyNoBundle       = usageerr.Usage(errors.New("one of SRC and DST must be a bundle path, as BUNDLE:PATH"))
	errCopyStdinName      = usageerr.Usage(errors.New("a copy from stdin needs a file name in DST"))
	errMoveBetweenBundles = usageerr.Usage(errors.New("moving from one bundle to another is not supported: copy with cp, then remove with rm"))
	errCopyVersion        = usageerr.Usage(errors.New("--version applies only when copying from a bundle"))
	errCopyWriteFlags     = usageerr.Usage(errors.New("--expected-version and --if-exists apply only when copying into a bundle"))
	errCopyIfExists       = usageerr.Usage(errors.New("--if-exists applies only to a copy with --recursive, or to a file copied within one bundle"))
	errCopyStdin          = usageerr.Usage(errors.New("--recursive cannot copy from stdin"))
	errCopyStdout         = usageerr.Usage(errors.New("--recursive cannot copy to stdout"))
	errMoveNoBundle       = usageerr.Usage(errors.New("both SRC and DST must be bundle paths, as BUNDLE:PATH"))
	errMoveRoot           = usageerr.Usage(errors.New("the bundle root cannot be moved"))
	errMergeFile          = usageerr.Usage(errors.New("--if-exists merge applies only to a directory"))
	errCopyFilterFlags    = usageerr.Usage(errors.New("--exclude, --include, --no-default-excludes and --no-gitignore apply only when copying a local directory into a bundle with --recursive"))
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

// BundleOutput is where an operation on bundle files writes. Result is the
// command's Renderer, which a copy, move, removal or sync publishes what it
// did through; Content is stdout for the raw bytes of a file (cat, and a copy
// to -), which no format changes; and Notes is stderr, for the progress and
// notes that are not the result.
type BundleOutput struct {
	Result  BundleEmitter
	Content io.Writer
	Notes   io.Writer
}

// BundleEmitter publishes a result, and the records of a stream (the rounds of
// a watching sync). cmd/cliout.Renderer is one.
type BundleEmitter interface {
	output.Emitter
	EmitEvent(v any, text func(w io.Writer) error) error
}

// BundleFileListing is the wire shape for `bundle files ls` output: the
// directory listed, the bundle version it was read at, and the directories
// (by name) and files (by path in the bundle) in it, [] when there are none.
type BundleFileListing struct {
	Path        string           `json:"path"`
	Version     *string          `json:"version,omitempty"`
	Directories []string         `json:"directories"`
	Files       []BundleFileInfo `json:"files"`
}

// BundleFileInfo is one file in a bundle: its path from the bundle root, its
// size in bytes and the sha256 digest of its content.
type BundleFileInfo struct {
	Path   string `json:"path"`
	Size   int    `json:"size"`
	Digest string `json:"digest"`
}

// BundleFileChange is what a copy, a move or a removal did, as it publishes
// it under --output json. Action is copied, moved or removed. Source and
// Destination are the paths as the command printed them (BUNDLE:PATH, or a
// local path); Version is the bundle version a write created, and
// SourceVersion the version the source was read at. Files counts the files a
// directory copy moved, and Skipped the local paths a directory copy left out.
// A field that does not apply stays out.
type BundleFileChange struct {
	Action        string `json:"action"`
	Source        string `json:"source,omitempty"`
	SourceVersion string `json:"source_version,omitempty"`
	Destination   string `json:"destination,omitempty"`
	Version       string `json:"version,omitempty"`
	Files         *int   `json:"files,omitempty"`
	Skipped       int    `json:"skipped,omitempty"`
}

// The actions a BundleFileChange publishes.
const (
	bundleFilesCopied  = "copied"
	bundleFilesMoved   = "moved"
	bundleFilesRemoved = "removed"
)

// publish publishes change, and in text the line text.
func (o BundleOutput) publish(change *BundleFileChange, text string) error {
	return o.Result.Emit(change, func(w io.Writer) error {
		_, err := fmt.Fprintln(w, text)
		return err
	})
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
			listing := d.(*BundleFileListing)
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
func ListBundleFilesData(ctx httpContext.Context, target BundlePath, recursive bool, version, wsID, deploymentID string, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) (*BundleFileListing, error) {
	b, err := resolveBundle(ctx, target, wsID, deploymentID, ToRead, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return nil, err
	}
	listing, err := listBundleFiles(ctx, target.Path, recursive, version, b, astroV1Alpha1Client)
	if err != nil {
		return nil, err
	}
	out := &BundleFileListing{Path: listing.Path, Version: listing.Version, Directories: listing.Directories, Files: make([]BundleFileInfo, 0, len(listing.Files))}
	if out.Directories == nil {
		out.Directories = []string{}
	}
	for _, f := range listing.Files {
		out.Files = append(out.Files, BundleFileInfo{Path: f.Path, Size: f.Size, Digest: f.Digest})
	}
	return out, nil
}

func listBundleFiles(ctx httpContext.Context, dir string, recursive bool, version string, b bundleRef, astroV1Alpha1Client astrov1alpha1.APIClient) (*astrov1alpha1.BundleFileList, error) {
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

	resp, err := astroV1Alpha1Client.ListBundleFilesWithResponse(pagination.Background(ctx), b.orgID, b.deploymentID, b.bundleID, params)
	if err != nil {
		return nil, err
	}
	err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	// A success with no listing in the body says nothing about the files, so
	// it is not taken for none.
	if resp.JSON200 == nil {
		return nil, errNoFileList
	}

	return resp.JSON200, nil
}

// ListBundleFilesWithFormat publishes a bundle's files in the shape the
// command's format asks for.
func ListBundleFilesWithFormat(ctx httpContext.Context, target BundlePath, recursive bool, version, wsID, deploymentID string, r output.Emitter, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	return output.PrintData(
		func() (*BundleFileListing, error) {
			return ListBundleFilesData(ctx, target, recursive, version, wsID, deploymentID, astroV1Client, astroV1Alpha1Client)
		},
		bundleFileTableConfig(), r,
	)
}

// CatBundleFile writes one file's content from a bundle to out, at the bundle's
// current version or at an earlier one.
func CatBundleFile(ctx httpContext.Context, target BundlePath, version, wsID, deploymentID string, out BundleOutput, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) error {
	if target.isDir() {
		return fmt.Errorf("%s is a directory", target)
	}
	b, err := resolveBundle(ctx, target, wsID, deploymentID, ToRead, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return err
	}
	content, _, err := openBundleFile(ctx, target.Path, version, b, astroV1Alpha1StreamClient)
	if err != nil {
		return err
	}
	defer content.Close()
	_, err = io.Copy(out.Content, content)
	return err
}

// RemoveBundlePath removes one file from a bundle, or a directory and everything
// under it with recursive, where the bundle root empties the bundle.
// A recursive removal asks for confirmation unless force is set.
// expectedVersion, when set, makes the removal conditional on the bundle still
// being at that version.
func RemoveBundlePath(ctx httpContext.Context, target BundlePath, recursive bool, expectedVersion, wsID, deploymentID string, force bool, out BundleOutput, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	if target.isDir() && !recursive {
		return fmt.Errorf("%s is a directory: remove it with --recursive", target)
	}
	b, err := resolveBundle(ctx, target, wsID, deploymentID, ToChange, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return err
	}

	if recursive && !force {
		prompt := fmt.Sprintf("Remove %s?", target)
		if target.Path == "" {
			prompt = fmt.Sprintf("Remove every file in %s?", target.Bundle)
		}
		if err := input.ConfirmOrAbort(prompt, input.AnsweredBy("--yes")); err != nil {
			return err
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
		resp, err := astroV1Alpha1Client.DeleteBundleDirectoryWithResponse(ctx, b.orgID, b.deploymentID, b.bundleID, params)
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
		resp, err := astroV1Alpha1Client.DeleteBundleFileWithResponse(ctx, b.orgID, b.deploymentID, b.bundleID, params)
		if err != nil {
			return err
		}
		err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return err
		}
		version = resp.JSON200.Version
	}

	return out.publish(&BundleFileChange{Action: bundleFilesRemoved, Source: target.String(), Version: version}, fmt.Sprintf("Removed %s at version %s", target, version))
}

// MoveBundlePath moves a file or a directory to another path in the same bundle.
func MoveBundlePath(ctx httpContext.Context, src, dst, ifExists, expectedVersion, wsID, deploymentID string, out BundleOutput, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
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

	b, dstBundle, err := resolveBundlePair(ctx, source, target, wsID, deploymentID, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return err
	}
	if dstBundle != b {
		return errMoveBetweenBundles
	}
	isDir, err := isBundleDirectory(ctx, source, expectedVersion, b, astroV1Alpha1Client)
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
		resp, err := astroV1Alpha1Client.MoveBundleDirectoryWithResponse(ctx, b.orgID, b.deploymentID, b.bundleID, request)
		if err != nil {
			return err
		}
		err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return err
		}
		version = resp.JSON200.Version
	} else {
		resp, err := astroV1Alpha1Client.MoveBundleFileWithResponse(ctx, b.orgID, b.deploymentID, b.bundleID, request)
		if err != nil {
			return err
		}
		err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return err
		}
		version = resp.JSON200.Version
	}

	return out.publish(&BundleFileChange{Action: bundleFilesMoved, Source: source.String(), Destination: target.String(), Version: version}, fmt.Sprintf("Moved %s to %s at version %s", source, target, version))
}

// CopyBundleFiles copies a file, or a directory's contents with Recursive, from a
// local path to a bundle path, from a bundle path to a local path, or between
// two bundle paths, on one Deployment or two. A local path of "-" is stdin or
// stdout.
func CopyBundleFiles(ctx httpContext.Context, src, dst string, opts *BundleCopyOptions, wsID, deploymentID string, out BundleOutput, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) error {
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
		return copyBundleToBundle(ctx, source, target, opts, wsID, deploymentID, out, astroV1Client, astroV1Alpha1Client, astroV1Alpha1StreamClient)
	case dstInBundle:
		if opts.Version != "" {
			return errCopyVersion
		}
		target, err := ParseBundlePath(dst)
		if err != nil {
			return err
		}
		return copyToBundle(ctx, src, target, opts, wsID, deploymentID, out, astroV1Client, astroV1Alpha1Client, astroV1Alpha1StreamClient)
	case srcInBundle:
		if opts.ExpectedVersion != "" || opts.IfExists != "" {
			return errCopyWriteFlags
		}
		source, err := ParseBundlePath(src)
		if err != nil {
			return err
		}
		return copyFromBundle(ctx, source, dst, opts, wsID, deploymentID, out, astroV1Client, astroV1Alpha1Client, astroV1Alpha1StreamClient)
	default:
		return errCopyNoBundle
	}
}

func copyToBundle(ctx httpContext.Context, src string, dst BundlePath, opts *BundleCopyOptions, wsID, deploymentID string, out BundleOutput, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) error {
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
		return writeBundleFile(ctx, content, "stdin", dst, opts.ExpectedVersion, wsID, deploymentID, out, astroV1Client, astroV1Alpha1Client)
	}

	info, err := os.Stat(src)
	if err != nil {
		return errors.Wrapf(err, "failed to read %s", src)
	}
	if info.IsDir() {
		if !opts.Recursive {
			return fmt.Errorf("%s is a directory: copy it with --recursive", src)
		}
		return writeBundleDirectory(ctx, src, dst, opts, wsID, deploymentID, out, astroV1Client, astroV1Alpha1Client, astroV1Alpha1StreamClient)
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
	return writeBundleFile(ctx, content, src, target, opts.ExpectedVersion, wsID, deploymentID, out, astroV1Client, astroV1Alpha1Client)
}

func copyFromBundle(ctx httpContext.Context, src BundlePath, dst string, opts *BundleCopyOptions, wsID, deploymentID string, out BundleOutput, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) error {
	if opts.Recursive {
		if dst == "-" {
			return errCopyStdout
		}
		return readBundleDirectory(ctx, src, dst, opts.Version, wsID, deploymentID, out, astroV1Client, astroV1Alpha1Client, astroV1Alpha1StreamClient)
	}
	if src.isDir() {
		return fmt.Errorf("%s is a directory: copy it with --recursive", src)
	}

	b, err := resolveBundle(ctx, src, wsID, deploymentID, ToRead, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return err
	}
	content, version, err := openBundleFile(ctx, src.Path, opts.Version, b, astroV1Alpha1StreamClient)
	if err != nil {
		return err
	}
	defer content.Close()
	if dst == "-" {
		_, err = io.Copy(out.Content, content)
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
	defer root.Close() //nolint:errcheck // a directory handle; nothing is flushed by closing it
	err = writeLocalFile(ctx, root, filepath.Base(dst), content)
	if err != nil {
		return err
	}

	return out.publish(&BundleFileChange{Action: bundleFilesCopied, Source: src.String(), SourceVersion: version, Destination: dst}, fmt.Sprintf("Copied %s at version %s to %s", src, version, dst))
}

// copyBundleToBundle copies within one bundle on the server, or else passes
// what it reads from SRC to the write to DST: a directory streams, and a file,
// at most 1 MiB, is read whole.
func copyBundleToBundle(ctx httpContext.Context, src, dst BundlePath, opts *BundleCopyOptions, wsID, deploymentID string, out BundleOutput, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) error {
	if !opts.Recursive {
		if src.isDir() {
			return fmt.Errorf("%s is a directory: copy it with --recursive", src)
		}
		if opts.IfExists == string(astrov1alpha1.MoveBundlePathRequestIfExistsMERGE) {
			return errMergeFile
		}
		dst = dst.into(path.Base(src.Path))
	}
	source, target, err := resolveBundlePair(ctx, src, dst, wsID, deploymentID, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return err
	}
	if source == target && opts.Version == "" {
		return copyWithinBundle(ctx, src, dst, source, opts, out, astroV1Alpha1Client)
	}
	if !opts.Recursive && opts.IfExists != "" {
		return errCopyIfExists
	}

	var readVersion, version string
	if opts.Recursive {
		next, archiveVersion, err := openBundleDirectory(ctx, src.Path, opts.Version, source, astroV1Alpha1StreamClient)
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
				archive, _, err = openBundleDirectory(ctx, src.Path, cmp.Or(readVersion, opts.Version), source, astroV1Alpha1StreamClient)
				if err != nil {
					return err
				}
			}
			defer archive.Close()
			_, err := io.Copy(w, archive)
			return err
		}
		version, err = putBundleDirectory(ctx, pack, bundleDirectoryParams(dst.Path, opts.IfExists, opts.ExpectedVersion), target, astroV1Alpha1StreamClient)
		if err != nil {
			return err
		}
	} else {
		content, contentVersion, err := openBundleFile(ctx, src.Path, opts.Version, source, astroV1Alpha1StreamClient)
		if err != nil {
			return err
		}
		defer content.Close()
		readVersion = contentVersion
		version, err = putBundleFile(ctx, content, dst.Path, opts.ExpectedVersion, target, astroV1Alpha1Client)
		if err != nil {
			return err
		}
	}

	return out.publish(&BundleFileChange{Action: bundleFilesCopied, Source: src.String(), SourceVersion: readVersion, Destination: dst.String(), Version: version}, fmt.Sprintf("Copied %s at version %s to %s at version %s", src, readVersion, dst, version))
}

func copyWithinBundle(ctx httpContext.Context, src, dst BundlePath, b bundleRef, opts *BundleCopyOptions, out BundleOutput, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	request := bundlePathRequest(src, dst, opts.IfExists, opts.ExpectedVersion)
	var version string
	if opts.Recursive {
		resp, err := astroV1Alpha1Client.DuplicateBundleDirectoryWithResponse(ctx, b.orgID, b.deploymentID, b.bundleID, request)
		if err != nil {
			return err
		}
		err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return err
		}
		version = resp.JSON200.Version
	} else {
		resp, err := astroV1Alpha1Client.DuplicateBundleFileWithResponse(ctx, b.orgID, b.deploymentID, b.bundleID, request)
		if err != nil {
			return err
		}
		err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return err
		}
		version = resp.JSON200.Version
	}

	return out.publish(&BundleFileChange{Action: bundleFilesCopied, Source: src.String(), Destination: dst.String(), Version: version}, fmt.Sprintf("Copied %s to %s at version %s", src, dst, version))
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
func isBundleDirectory(ctx httpContext.Context, target BundlePath, version string, b bundleRef, astroV1Alpha1Client astrov1alpha1.APIClient) (bool, error) {
	parent := path.Dir(target.Path)
	if parent == "." {
		parent = ""
	}
	listing, err := listBundleFiles(ctx, parent, false, version, b, astroV1Alpha1Client)
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
func openBundleFile(ctx httpContext.Context, filePath, version string, b bundleRef, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) (content io.ReadCloser, readVersion string, err error) {
	params := &astrov1alpha1.ReadBundleFileParams{Path: filePath}
	if version != "" {
		params.Version = &version
	}

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
func openBundleDirectory(ctx httpContext.Context, dir, version string, b bundleRef, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) (archive io.ReadCloser, readVersion string, err error) {
	params := &astrov1alpha1.ReadBundleDirectoryParams{}
	if dir != "" {
		params.Path = &dir
	}
	if version != "" {
		params.Version = &version
	}

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

func writeBundleFile(ctx httpContext.Context, content io.Reader, src string, dst BundlePath, expectedVersion, wsID, deploymentID string, out BundleOutput, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	b, err := resolveBundle(ctx, dst, wsID, deploymentID, ToChange, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return err
	}
	version, err := putBundleFile(ctx, content, dst.Path, expectedVersion, b, astroV1Alpha1Client)
	if err != nil {
		return err
	}

	return out.publish(&BundleFileChange{Action: bundleFilesCopied, Source: src, Destination: dst.String(), Version: version}, fmt.Sprintf("Copied %s to %s at version %s", src, dst, version))
}

// putBundleFile writes content to filePath in a bundle and returns the version the write created.
func putBundleFile(ctx httpContext.Context, content io.Reader, filePath, expectedVersion string, b bundleRef, astroV1Alpha1Client astrov1alpha1.APIClient) (string, error) {
	params := &astrov1alpha1.WriteBundleFileParams{Path: filePath}
	if expectedVersion != "" {
		params.ExpectedVersion = &expectedVersion
	}

	resp, err := astroV1Alpha1Client.WriteBundleFileWithBodyWithResponse(pagination.Background(ctx), b.orgID, b.deploymentID, b.bundleID, params, bundleFileContentType, content)
	if err != nil {
		return "", err
	}
	err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return "", err
	}
	return resp.JSON200.Version, nil
}

func readBundleDirectory(ctx httpContext.Context, src BundlePath, dst, version, wsID, deploymentID string, out BundleOutput, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) error {
	b, err := resolveBundle(ctx, src, wsID, deploymentID, ToRead, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return err
	}
	archive, readVersion, err := openBundleDirectory(ctx, src.Path, version, b, astroV1Alpha1StreamClient)
	if err != nil {
		return err
	}
	defer archive.Close()
	files, err := unpackBundleDirectory(ctx, archive, dst, nil)
	if err != nil {
		return err
	}

	return out.publish(&BundleFileChange{Action: bundleFilesCopied, Source: src.String(), SourceVersion: readVersion, Destination: dst, Files: &files}, fmt.Sprintf("Copied %d files from %s at version %s to %s", files, src, readVersion, dst))
}

func writeBundleDirectory(ctx httpContext.Context, src string, dst BundlePath, opts *BundleCopyOptions, wsID, deploymentID string, out BundleOutput, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) error {
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

	b, err := resolveBundle(ctx, dst, wsID, deploymentID, ToChange, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return err
	}
	version, err := uploadBundleDirectory(ctx, src, &bundleSyncPlan{added: files}, bundleDirectoryParams(dst.Path, opts.IfExists, opts.ExpectedVersion), b, astroV1Alpha1StreamClient)
	if err != nil {
		return err
	}

	printSkipped(out.Notes, skipped)
	count := len(files)
	return out.publish(&BundleFileChange{Action: bundleFilesCopied, Source: src, Destination: dst.String(), Version: version, Files: &count, Skipped: skipped}, fmt.Sprintf("Copied %d files from %s to %s at version %s", count, src, dst, version))
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
func putBundleDirectory(ctx httpContext.Context, pack func(io.Writer) error, params *astrov1alpha1.WriteBundleDirectoryParams, b bundleRef, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) (string, error) {
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
		return "", &bundleWriteError{err: err}
	}
	parsed, err := astrov1alpha1.ParseWriteBundleDirectoryResponse(resp)
	if err != nil {
		return "", &bundleWriteError{err: err}
	}
	err = astrov1alpha1.NormalizeAPIError(parsed.HTTPResponse, parsed.Body)
	if err != nil {
		return "", &bundleWriteError{status: parsed.StatusCode(), err: err}
	}
	return parsed.JSON200.Version, nil
}

// bundleSyncPlan is what a write to a bundle directory changes, by path
// relative to the directory: a copy of a local directory adds every file, and a
// sync adds, updates and deletes the ones that differ.
type bundleSyncPlan struct {
	added, updated, deleted []string
}

// bundleWriteError is a failed write to a bundle directory, with the status the API answered, or 0 if it did not answer.
type bundleWriteError struct {
	status int
	err    error
}

func (e *bundleWriteError) Error() string { return e.err.Error() }

func (e *bundleWriteError) Unwrap() error { return e.err }

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
	b.PipeReader.Close() //nolint:errcheck // closing a pipe reader always succeeds
	return b.wait()
}

// uploadBundleDirectory applies plan from the files under localDir to a bundle
// directory as a gzipped tar, and returns the version the write created.
func uploadBundleDirectory(ctx httpContext.Context, localDir string, plan *bundleSyncPlan, params *astrov1alpha1.WriteBundleDirectoryParams, b bundleRef, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) (string, error) {
	pack := func(w io.Writer) error {
		return packBundleDirectory(ctx, w, localDir, plan)
	}
	return putBundleDirectory(ctx, pack, params, b, astroV1Alpha1StreamClient)
}

// resolveDeployment returns the Deployment whose ID a bundle path holds, else
// the one deploymentID names, else the one the user selects.
//
// A Deployment named in a path is an id and nothing else: a path has no room
// for the link names and Deployment names the --deployment flag takes, and
// every colon in it separates the Deployment from the bundle. use is what the
// caller does with it, as for GetDeployment: a Deployment named by the flag, or
// by nothing, is chosen as that use allows, and one named in a path is the
// one named.
func resolveDeployment(ctx httpContext.Context, pathDeploymentID, wsID, deploymentID string, use Use, astroV1Client astrov1.APIClient) (astrov1.Deployment, error) {
	if pathDeploymentID != "" {
		return GetDeploymentByID(ctx, "", pathDeploymentID, astroV1Client)
	}
	return GetDeploymentContext(ctx, wsID, deploymentID, "", use, nil, astroV1Client)
}

// resolveBundle resolves the bundle of target, named by its ID or its Dag bundle name.
func resolveBundle(ctx httpContext.Context, target BundlePath, wsID, deploymentID string, use Use, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) (bundleRef, error) {
	dep, err := resolveDeployment(ctx, target.Deployment, wsID, deploymentID, use, astroV1Client)
	if err != nil {
		return bundleRef{}, err
	}
	bundles, err := listAllBundles(ctx, dep.OrganizationId, dep.Id, astroV1Alpha1Client)
	if err != nil {
		return bundleRef{}, err
	}
	bundleID, err := findBundleID(bundles, target, dep.Id)
	if err != nil {
		return bundleRef{}, err
	}
	return bundleRef{orgID: dep.OrganizationId, deploymentID: dep.Id, bundleID: bundleID}, nil
}

// resolveBundlePair resolves the bundles of src and dst, looking up a Deployment they share once.
// It is for a write to dst, so both Deployments are looked up to change.
func resolveBundlePair(ctx httpContext.Context, src, dst BundlePath, wsID, deploymentID string, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) (source, target bundleRef, err error) {
	srcDep, err := resolveDeployment(ctx, src.Deployment, wsID, deploymentID, ToChange, astroV1Client)
	if err != nil {
		return bundleRef{}, bundleRef{}, err
	}
	srcBundles, err := listAllBundles(ctx, srcDep.OrganizationId, srcDep.Id, astroV1Alpha1Client)
	if err != nil {
		return bundleRef{}, bundleRef{}, err
	}
	dstDep, dstBundles := srcDep, srcBundles
	if dst.Deployment != src.Deployment {
		dstDep, err = resolveDeployment(ctx, dst.Deployment, wsID, deploymentID, ToChange, astroV1Client)
		if err != nil {
			return bundleRef{}, bundleRef{}, err
		}
		if dstDep.Id != srcDep.Id {
			dstBundles, err = listAllBundles(ctx, dstDep.OrganizationId, dstDep.Id, astroV1Alpha1Client)
			if err != nil {
				return bundleRef{}, bundleRef{}, err
			}
		}
	}

	srcBundleID, err := findBundleID(srcBundles, src, srcDep.Id)
	if err != nil {
		return bundleRef{}, bundleRef{}, err
	}
	dstBundleID, err := findBundleID(dstBundles, dst, dstDep.Id)
	if err != nil {
		return bundleRef{}, bundleRef{}, err
	}
	source = bundleRef{orgID: srcDep.OrganizationId, deploymentID: srcDep.Id, bundleID: srcBundleID}
	target = bundleRef{orgID: dstDep.OrganizationId, deploymentID: dstDep.Id, bundleID: dstBundleID}
	return source, target, nil
}

// findBundleID returns the ID of the bundle whose ID or, failing that, whose name is target's bundle.
func findBundleID(bundles []astrov1alpha1.DeploymentBundle, target BundlePath, deploymentID string) (string, error) {
	ref := target.Bundle
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
	notFound := fmt.Sprintf("no bundle with the ID or name %q on deployment %s", ref, deploymentID)
	if target.Deployment == "" && util.IsCUID(ref) && target.Path != "" && !strings.Contains(target.Path, "/") {
		return "", fmt.Errorf("%s: did you mean %q?", notFound, ref+":"+target.Path+":")
	}
	return "", errors.New(notFound)
}

// packBundleDirectory writes to archive a gzipped tar that applies plan, by
// slash-separated paths relative to localDir: it deletes, then writes the files
// added and updated. The deletes come first, so a file can take the place of a
// directory.
func packBundleDirectory(ctx httpContext.Context, archive io.Writer, localDir string, plan *bundleSyncPlan) error {
	gzipWriter := gzip.NewWriter(archive)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, relPath := range plan.deleted {
		err := tarWriter.WriteHeader(&tar.Header{Name: bundleDeletePrefix + relPath, Mode: 0o644, Typeflag: tar.TypeReg})
		if err != nil {
			return err
		}
	}
	for _, relPath := range slices.Concat(plan.added, plan.updated) {
		file, err := os.Open(filepath.Join(localDir, filepath.FromSlash(relPath)))
		if err != nil {
			return errors.Wrapf(err, "failed to read %s", relPath)
		}
		info, err := file.Stat()
		if err == nil {
			err = tarWriter.WriteHeader(&tar.Header{Name: relPath, Mode: 0o644, Size: info.Size(), Typeflag: tar.TypeReg})
		}
		if err == nil {
			_, err = io.Copy(tarWriter, ctxReader{ctx: ctx, r: file})
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
func unpackBundleDirectory(ctx httpContext.Context, archive io.Reader, localDir string, keep func(relPath string) bool) (int, error) {
	err := os.MkdirAll(localDir, os.ModePerm)
	if err != nil {
		return 0, err
	}
	root, err := os.OpenRoot(localDir)
	if err != nil {
		return 0, err
	}
	defer root.Close() //nolint:errcheck // a directory handle; nothing is flushed by closing it

	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read the bundle directory")
	}
	defer gzipReader.Close() //nolint:errcheck // a reader; the tar's own errors are what matter

	tarReader := tar.NewReader(gzipReader)
	// Each file written, by its path in lower case, to catch two that a case-insensitive file system would make one.
	written := map[string]string{}
	files := 0
	for {
		if err := ctx.Err(); err != nil {
			return files, err
		}
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
		err = writeLocalFile(ctx, root, name, tarReader)
		if err != nil {
			return files, err
		}
		written[lower] = header.Name
		files++
	}
}

// ctxReader reads from r until ctx ends, so a copy of a local file stops with
// the run's interrupt instead of finishing a large file first.
type ctxReader struct {
	ctx httpContext.Context //nolint:containedctx // a reader bound to one copy
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
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
func writeLocalFile(ctx httpContext.Context, root *os.Root, name string, content io.Reader) error {
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
	_, err = io.Copy(file, ctxReader{ctx: ctx, r: content})
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = root.Rename(temp, name)
	}
	if err != nil {
		root.Remove(temp) //nolint:errcheck // best effort: the write's own error is what is returned
		return err
	}
	return nil
}

// NeedsDeploymentScope reports whether an operation on args needs the run to
// say which Deployment it acts on: some argument is a bundle path that names no
// Deployment of its own, or none is a bundle path, which the operation then
// refuses itself. A command whose every bundle path is DEPLOYMENT_ID:BUNDLE:PATH
// looks no Workspace up.
func NeedsDeploymentScope(args ...string) bool {
	anyBundle := false
	for _, arg := range args {
		if !isBundlePath(arg) {
			continue
		}
		anyBundle = true
		if p, err := ParseBundlePath(arg); err != nil || p.Deployment == "" {
			return true
		}
	}
	return !anyBundle
}
