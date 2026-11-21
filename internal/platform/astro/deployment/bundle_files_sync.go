package deployment

import (
	httpContext "context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pkg/errors"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	"github.com/astronomer/astro-cli/internal/usageerr"
	"github.com/astronomer/astro-cli/pkg/input"
)

const bundleDigestPrefix = "sha256:"

var (
	errSyncNoBundle        = usageerr.Usage(errors.New("one of SRC and DST must be a bundle path, as BUNDLE:PATH, and the other a local directory"))
	errSyncVersion         = usageerr.Usage(errors.New("--version applies only when syncing from a bundle"))
	errSyncExpectedVersion = usageerr.Usage(errors.New("--expected-version applies only when syncing to a bundle"))
	errSyncYes             = usageerr.Usage(errors.New("--yes applies only when syncing from a bundle"))
)

// BundleSyncOptions are the flags of a sync between a local directory and a
// bundle directory.
type BundleSyncOptions struct {
	NoDelete bool
	// Yes deletes local files in a sync from a bundle without asking first.
	Yes             bool
	DryRun          bool
	Watch           bool
	Version         string
	ExpectedVersion string
	Filter          BundleFilterOptions
}

// The directions of a sync, as a BundleSyncResult names them.
const (
	syncUpload   = "upload"
	syncDownload = "download"
)

// BundleSyncResult is what a sync did, or with --dry-run would do, as it
// publishes it under --output json. Added, Updated and Deleted are the paths
// the sync changed in DST, relative to the synced directories, [] when there
// are none. Version is the bundle version the sync wrote, or else the one it
// read: the bundle's current version for an upload that had nothing to do, the
// version a download came from. Changed is whether anything was written, and
// Skipped counts the local paths the filter left out. A watching sync
// publishes one of these for each time it wrote.
type BundleSyncResult struct {
	Source      string   `json:"source"`
	Destination string   `json:"destination"`
	Direction   string   `json:"direction"`
	DryRun      bool     `json:"dry_run"`
	Changed     bool     `json:"changed"`
	Version     string   `json:"version,omitempty"`
	Added       []string `json:"added"`
	Updated     []string `json:"updated"`
	Deleted     []string `json:"deleted"`
	Skipped     int      `json:"skipped,omitempty"`
}

func newSyncResult(src, dst, direction string, plan *bundleSyncPlan, skipped int, dryRun bool) *BundleSyncResult {
	return &BundleSyncResult{
		Source:      src,
		Destination: dst,
		Direction:   direction,
		DryRun:      dryRun,
		Added:       append([]string{}, plan.added...),
		Updated:     append([]string{}, plan.updated...),
		Deleted:     append([]string{}, plan.deleted...),
		Skipped:     skipped,
	}
}

// publishSync publishes result, and in text the changes plan lists, by the
// names name gives them, then line. A watching sync publishes each round as a
// record of a stream (event).
func (o BundleOutput) publishSync(result *BundleSyncResult, plan *bundleSyncPlan, name func(relPath string) string, line string, event bool) error {
	text := func(w io.Writer) error {
		plan.print(w, name)
		_, err := fmt.Fprintln(w, line)
		return err
	}
	if event {
		return o.Result.EmitEvent(result, text)
	}
	return o.Result.Emit(result, text)
}

// localDigests returns the digest of each file, by its path relative to localDir.
func localDigests(localDir string, files []string) (map[string]string, error) {
	digests := make(map[string]string, len(files))
	for _, relPath := range files {
		file, err := os.Open(filepath.Join(localDir, filepath.FromSlash(relPath)))
		if err != nil {
			return nil, err
		}
		hash := sha256.New()
		_, err = io.Copy(hash, file)
		closeErr := file.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		digests[relPath] = bundleDigestPrefix + hex.EncodeToString(hash.Sum(nil))
	}
	return digests, nil
}

// bundleDigests returns the digest of each file in a recursive listing of dir
// that filter keeps, by its path relative to dir, refusing a dir that is a file.
func bundleDigests(listing *astrov1alpha1.BundleFileList, dir BundlePath, filter *localFileFilter) (map[string]string, error) {
	digests := make(map[string]string, len(listing.Files))
	for _, file := range listing.Files {
		if file.Path == dir.Path {
			return nil, fmt.Errorf("%s is a file: sync takes a directory", dir)
		}
		relPath := file.Path
		if dir.Path != "" {
			relPath = strings.TrimPrefix(file.Path, dir.Path+"/")
		}
		if !filter.excluded(relPath) {
			digests[relPath] = file.Digest
		}
	}
	return digests, nil
}

func planBundleSync(from, to map[string]string, deleteExtras bool) *bundleSyncPlan {
	plan := &bundleSyncPlan{}
	for relPath, digest := range from {
		toDigest, found := to[relPath]
		switch {
		case !found:
			plan.added = append(plan.added, relPath)
		case toDigest != digest:
			plan.updated = append(plan.updated, relPath)
		}
	}
	if deleteExtras {
		for relPath := range to {
			if _, found := from[relPath]; !found {
				plan.deleted = append(plan.deleted, relPath)
			}
		}
	}
	slices.Sort(plan.added)
	slices.Sort(plan.updated)
	slices.Sort(plan.deleted)
	return plan
}

func (p *bundleSyncPlan) empty() bool {
	return len(p.added) == 0 && len(p.updated) == 0 && len(p.deleted) == 0
}

func (p *bundleSyncPlan) print(out io.Writer, name func(relPath string) string) {
	for _, relPath := range p.added {
		fmt.Fprintf(out, "add     %s\n", name(relPath))
	}
	for _, relPath := range p.updated {
		fmt.Fprintf(out, "update  %s\n", name(relPath))
	}
	for _, relPath := range p.deleted {
		fmt.Fprintf(out, "delete  %s\n", name(relPath))
	}
}

func (p *bundleSyncPlan) summary() string {
	return fmt.Sprintf("%d added, %d updated, %d deleted", len(p.added), len(p.updated), len(p.deleted))
}

// SyncBundleFiles makes DST match SRC, one a local directory and the other a bundle directory, transferring only the files that differ.
// With opts.Watch, it then syncs again each time files under SRC change, until ctx is done or the process is interrupted.
func SyncBundleFiles(ctx httpContext.Context, src, dst string, opts *BundleSyncOptions, wsID, deploymentID string, out BundleOutput, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) error {
	srcInBundle, dstInBundle := isBundlePath(src), isBundlePath(dst)
	if srcInBundle == dstInBundle || src == "-" || dst == "-" {
		return errSyncNoBundle
	}
	if opts.Watch && opts.DryRun {
		return errSyncWatchDryRun
	}
	if dstInBundle {
		if opts.Version != "" {
			return errSyncVersion
		}
		if opts.Yes {
			return errSyncYes
		}
		target, err := ParseBundlePath(dst)
		if err != nil {
			return err
		}
		return syncToBundle(ctx, src, target, opts, wsID, deploymentID, out, astroV1Client, astroV1Alpha1Client, astroV1Alpha1StreamClient)
	}
	if opts.ExpectedVersion != "" {
		return errSyncExpectedVersion
	}
	if opts.Watch {
		return errSyncWatch
	}
	source, err := ParseBundlePath(src)
	if err != nil {
		return err
	}
	return syncFromBundle(ctx, source, dst, opts, wsID, deploymentID, out, astroV1Client, astroV1Alpha1Client, astroV1Alpha1StreamClient)
}

func syncToBundle(ctx httpContext.Context, src string, dst BundlePath, opts *BundleSyncOptions, wsID, deploymentID string, out BundleOutput, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) error {
	src, err := followLocalDir(src)
	if err != nil {
		return err
	}
	info, err := os.Stat(src)
	if err != nil {
		return errors.Wrapf(err, "failed to read %s", src)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", src)
	}
	filter, err := newLocalFileFilter(src, opts.Filter)
	if err != nil {
		return err
	}
	if opts.Watch {
		target, err := openSyncTarget(ctx, dst, opts.ExpectedVersion, filter, wsID, deploymentID, astroV1Client, astroV1Alpha1Client)
		if err != nil {
			return err
		}
		return target.watch(ctx, src, opts.Filter, filter, !opts.NoDelete, out, astroV1Alpha1StreamClient)
	}

	files, skipped, err := localFiles(src, filter)
	if err != nil {
		return err
	}
	err = checkBundleUpload(src, dst.Path, files)
	if err != nil {
		return err
	}
	local, err := localDigests(src, files)
	if err != nil {
		return err
	}
	target, err := openSyncTarget(ctx, dst, opts.ExpectedVersion, filter, wsID, deploymentID, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return err
	}
	plan := target.plan(local, !opts.NoDelete)
	printSkipped(out.Notes, skipped)

	result := newSyncResult(src, dst.String(), syncUpload, plan, skipped, opts.DryRun)
	if opts.DryRun || plan.empty() {
		result.Version = target.version
		return out.publishSync(result, plan, target.name, syncUnchangedLine(src, dst.String(), target.version, plan, opts.DryRun), false)
	}
	version, err := target.write(ctx, src, local, plan, astroV1Alpha1StreamClient)
	if err != nil {
		return err
	}
	result.Changed, result.Version = true, version
	return out.publishSync(result, plan, target.name, syncedLine(src, dst, version, plan), false)
}

// openSyncTarget resolves the bundle directory a sync writes to, and lists what it holds at expectedVersion, or now.
func openSyncTarget(ctx httpContext.Context, dst BundlePath, expectedVersion string, filter *localFileFilter, wsID, deploymentID string, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) (*bundleSyncTarget, error) {
	b, err := resolveBundle(ctx, dst, wsID, deploymentID, ToChange, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return nil, err
	}
	listing, err := listBundleFiles(ctx, dst.Path, true, expectedVersion, b, astroV1Alpha1Client)
	if err != nil {
		return nil, err
	}
	digests, err := bundleDigests(listing, dst, filter)
	if err != nil {
		return nil, err
	}
	target := &bundleSyncTarget{path: dst, ref: b, digests: digests}
	if listing.Version != nil {
		target.version = *listing.Version
	}
	return target, nil
}

// bundleSyncTarget is the bundle directory a sync writes to, with the digests
// of the files it holds at version.
type bundleSyncTarget struct {
	path    BundlePath
	ref     bundleRef
	digests map[string]string
	version string
}

func (t *bundleSyncTarget) name(relPath string) string {
	return BundlePath{Deployment: t.path.Deployment, Bundle: t.path.Bundle, Path: path.Join(t.path.Path, relPath)}.String()
}

// plan is what an upload of the local files with digests local changes in t.
func (t *bundleSyncTarget) plan(local map[string]string, deleteExtras bool) *bundleSyncPlan {
	return planBundleSync(local, t.digests, deleteExtras)
}

// write applies plan from the local files under src as one merging write, which
// fails if the bundle is no longer at t.version, and returns the version it
// created.
func (t *bundleSyncTarget) write(ctx httpContext.Context, src string, local map[string]string, plan *bundleSyncPlan, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) (string, error) {
	params := bundleDirectoryParams(t.path.Path, string(astrov1alpha1.WriteBundleDirectoryParamsIfExistsMERGE), t.version)
	version, err := uploadBundleDirectory(ctx, src, plan, params, t.ref, astroV1Alpha1StreamClient)
	if err != nil {
		return "", err
	}
	for _, relPath := range plan.deleted {
		delete(t.digests, relPath)
	}
	for _, relPath := range slices.Concat(plan.added, plan.updated) {
		t.digests[relPath] = local[relPath]
	}
	t.version = version
	return version, nil
}

// syncedLine is what a sync that wrote into a bundle says.
func syncedLine(src string, dst BundlePath, version string, plan *bundleSyncPlan) string {
	return fmt.Sprintf("Synced %s to %s at version %s: %s", src, dst, version, plan.summary())
}

func syncFromBundle(ctx httpContext.Context, src BundlePath, dst string, opts *BundleSyncOptions, wsID, deploymentID string, out BundleOutput, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) error {
	dst, err := followLocalDir(dst)
	if err != nil {
		return err
	}
	info, err := os.Stat(dst)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return errors.Wrapf(err, "failed to read %s", dst)
	case !info.IsDir():
		return fmt.Errorf("%s is not a directory", dst)
	}
	filter, err := newLocalFileFilter(dst, opts.Filter)
	if err != nil {
		return err
	}
	local := map[string]string{}
	skipped := 0
	if info != nil {
		var files []string
		files, skipped, err = localFiles(dst, filter)
		if err != nil {
			return err
		}
		local, err = localDigests(dst, files)
		if err != nil {
			return err
		}
	}

	b, err := resolveBundle(ctx, src, wsID, deploymentID, ToRead, astroV1Client, astroV1Alpha1Client)
	if err != nil {
		return err
	}
	listing, err := listBundleFiles(ctx, src.Path, true, opts.Version, b, astroV1Alpha1Client)
	if err != nil {
		return err
	}
	// A bundle directory exists only while it holds a file, so an empty listing is a path the bundle does not have.
	if src.Path != "" && len(listing.Files) == 0 {
		return fmt.Errorf("%s holds no files", src)
	}
	remote, err := bundleDigests(listing, src, filter)
	if err != nil {
		return err
	}
	plan := planBundleSync(remote, local, !opts.NoDelete)
	localName := func(relPath string) string {
		return filepath.Join(dst, filepath.FromSlash(relPath))
	}
	printSkipped(out.Notes, skipped)

	version := ""
	if listing.Version != nil {
		version = *listing.Version
	}
	result := newSyncResult(src.String(), dst, syncDownload, plan, skipped, opts.DryRun)
	result.Version = version
	if opts.DryRun || plan.empty() {
		return out.publishSync(result, plan, localName, syncUnchangedLine(src.String(), dst, version, plan, opts.DryRun), false)
	}

	transfers := slices.Concat(plan.added, plan.updated)
	blocking, err := blockingDeletes(dst, plan.deleted, transfers)
	if err != nil {
		return err
	}
	if len(plan.deleted) > 0 && !opts.Yes {
		if err := input.ConfirmOrAbort(fmt.Sprintf("Delete %d files in %s?", len(plan.deleted), dst), input.AnsweredBy("--yes")); err != nil {
			return err
		}
	}
	err = applyDownload(ctx, src.Path, version, dst, plan, blocking, b, astroV1Alpha1StreamClient)
	if err != nil {
		return err
	}

	result.Changed = true
	return out.publishSync(result, plan, localName, fmt.Sprintf("Synced %s at version %s to %s: %s", src, version, dst, plan.summary()), false)
}

// applyDownload applies plan to localDir from dir in a bundle at version. The
// download starts before anything changes, so a request that fails changes
// nothing; then the deletes in blocking make way for the transfers, which are
// written before the other deletes.
func applyDownload(ctx httpContext.Context, dir, version, localDir string, plan *bundleSyncPlan, blocking map[string]bool, b bundleRef, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) error {
	transfers := slices.Concat(plan.added, plan.updated)
	if len(transfers) > 0 {
		archive, _, err := openBundleDirectory(ctx, dir, version, b, astroV1Alpha1StreamClient)
		if err != nil {
			return err
		}
		defer archive.Close()
		for relPath := range blocking {
			err = removeLocalFile(localDir, relPath)
			if err != nil {
				return err
			}
		}
		wanted := make(map[string]bool, len(transfers))
		for _, relPath := range transfers {
			wanted[relPath] = true
		}
		_, err = unpackBundleDirectory(ctx, archive, localDir, func(relPath string) bool {
			return wanted[relPath]
		})
		if err != nil {
			return err
		}
	}
	for _, relPath := range plan.deleted {
		if blocking[relPath] {
			continue
		}
		err := removeLocalFile(localDir, relPath)
		if err != nil {
			return err
		}
	}
	return nil
}

// blockingDeletes returns the deletes of a download that must come before its
// transfers are written under localDir: a file where a transfer needs a
// directory, each file in a directory a transfer replaces, and a file whose
// path differs from a transfer's only in case, which is the same file where
// case is ignored. It fails, before anything changes, when a file the sync
// keeps is in a transfer's way.
func blockingDeletes(localDir string, deleted, transfers []string) (map[string]bool, error) {
	exact := make(map[string]bool, len(deleted))
	folded := make(map[string]string, len(deleted))
	for _, relPath := range deleted {
		exact[relPath] = true
		folded[strings.ToLower(relPath)] = relPath
	}
	// findDelete returns the delete of relPath, matching it in any case, since the file system may ignore case.
	findDelete := func(relPath string) (string, bool) {
		if exact[relPath] {
			return relPath, true
		}
		match, found := folded[strings.ToLower(relPath)]
		return match, found
	}
	blocking := map[string]bool{}
	block := func(relPath, transfer string) error {
		match, found := findDelete(relPath)
		if !found {
			return fmt.Errorf("%s must be removed to write %s, but the sync keeps it, because it is excluded or --no-delete is set", filepath.Join(localDir, filepath.FromSlash(relPath)), filepath.Join(localDir, filepath.FromSlash(transfer)))
		}
		blocking[match] = true
		return nil
	}
	for _, transfer := range transfers {
		match, found := findDelete(transfer)
		if found {
			blocking[match] = true
		}
		for dir := path.Dir(transfer); dir != "."; dir = path.Dir(dir) {
			info, err := os.Lstat(filepath.Join(localDir, filepath.FromSlash(dir)))
			if err == nil && !info.IsDir() {
				err = block(dir, transfer)
				if err != nil {
					return nil, err
				}
			}
		}
		target := filepath.Join(localDir, filepath.FromSlash(transfer))
		info, err := os.Lstat(target)
		if err != nil || !info.IsDir() {
			continue
		}
		err = filepath.WalkDir(target, func(filePath string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			relPath, err := filepath.Rel(localDir, filePath)
			if err != nil {
				return err
			}
			return block(filepath.ToSlash(relPath), transfer)
		})
		if err != nil {
			return nil, err
		}
	}
	return blocking, nil
}

// removeLocalFile removes the file at relPath under localDir, and then each of
// its parent directories below localDir that the removal left empty.
func removeLocalFile(localDir, relPath string) error {
	err := os.Remove(filepath.Join(localDir, filepath.FromSlash(relPath)))
	if err != nil {
		return err
	}
	for dir := path.Dir(relPath); dir != "."; dir = path.Dir(dir) {
		if os.Remove(filepath.Join(localDir, filepath.FromSlash(dir))) != nil {
			return nil
		}
	}
	return nil
}

// syncUnchangedLine reports a dry run, or a sync with nothing to do.
func syncUnchangedLine(src, dst, version string, plan *bundleSyncPlan, dryRun bool) string {
	at := ""
	if version != "" {
		at = " at bundle version " + version
	}
	if dryRun {
		return fmt.Sprintf("Dry run%s: %s; nothing was changed", at, plan.summary())
	}
	return fmt.Sprintf("%s is up to date with %s%s", dst, src, at)
}
