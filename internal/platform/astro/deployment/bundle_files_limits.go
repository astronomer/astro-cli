package deployment

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pkg/errors"
)

// The API's limits on file bundles, which an upload is checked against before it is sent.
const (
	maxBundlePathBytes  = 192
	maxBundleFileBytes  = 1 << 20
	maxBundleFiles      = 10000
	maxBundleBytes      = 100 << 20
	maxShownLimitFaults = 20
)

// maxArchivePaddingBytes is the API's limit on what a gzipped tar holds past the end of the tar, which a download reads no more than.
const maxArchivePaddingBytes = 64 << 10

// bundleLimitError lists what breaks the bundle limits in an upload that was not sent.
type bundleLimitError struct {
	faults []string
}

func (e *bundleLimitError) Error() string {
	shown := e.faults[:min(len(e.faults), maxShownLimitFaults)]
	msg := "nothing was written, because these break the bundle limits:\n  " + strings.Join(shown, "\n  ")
	if len(e.faults) > len(shown) {
		msg += fmt.Sprintf("\n  and %d more", len(e.faults)-len(shown))
	}
	return msg
}

// checkBundleUpload checks the files under localDir, by slash-separated path
// relative to it, that an upload writes under dir in a bundle.
func checkBundleUpload(localDir, dir string, files []string) error {
	if len(files) > maxBundleFiles {
		return &bundleLimitError{faults: []string{fmt.Sprintf("%s holds %d files, past the %d a bundle may hold", localDir, len(files), maxBundleFiles)}}
	}
	var faults []string
	var total int64
	for _, relPath := range files {
		localPath := filepath.Join(localDir, filepath.FromSlash(relPath))
		info, err := os.Stat(localPath)
		if err != nil {
			return err
		}
		total += info.Size()
		faults = append(faults, bundleFileFaults(localPath, path.Join(dir, relPath), info.Size())...)
	}
	if total > maxBundleBytes {
		faults = slices.Insert(faults, 0, fmt.Sprintf("%s holds %d bytes, past the %d a bundle may hold", localDir, total, maxBundleBytes))
	}
	if len(faults) > 0 {
		return &bundleLimitError{faults: faults}
	}
	return nil
}

// checkBundleFile checks one local file of size bytes that an upload writes to bundlePath.
func checkBundleFile(localPath, bundlePath string, size int64) error {
	faults := bundleFileFaults(localPath, bundlePath, size)
	if len(faults) > 0 {
		return &bundleLimitError{faults: faults}
	}
	return nil
}

// readBundleUpload reads the content of one file an upload writes, refusing more than a bundle file may hold.
func readBundleUpload(content io.Reader, localPath string) (*bytes.Reader, error) {
	data, err := io.ReadAll(io.LimitReader(content, maxBundleFileBytes+1))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %s", localPath)
	}
	if len(data) > maxBundleFileBytes {
		return nil, &bundleLimitError{faults: []string{fmt.Sprintf("%s holds more than the %d bytes a bundle file may hold", localPath, maxBundleFileBytes)}}
	}
	return bytes.NewReader(data), nil
}

func bundleFileFaults(localPath, bundlePath string, size int64) []string {
	var faults []string
	if size > maxBundleFileBytes {
		faults = append(faults, fmt.Sprintf("%s is %d bytes, past the %d a bundle file may hold", localPath, size, maxBundleFileBytes))
	}
	if fault := bundlePathFault(bundlePath); fault != "" {
		faults = append(faults, fmt.Sprintf("%s: its bundle path %s %s", localPath, bundlePath, fault))
	}
	return faults
}

// bundlePathFault says why a bundle cannot hold a file at p, by the API's rules, or returns "" if it can.
func bundlePathFault(p string) string {
	if len(p) > maxBundlePathBytes {
		return fmt.Sprintf("is %d bytes, past the %d allowed", len(p), maxBundlePathBytes)
	}
	if strings.ContainsFunc(p, func(r rune) bool { return !isBundlePathRune(r) }) {
		return "may hold only ASCII letters, digits, '/', '.', '_' and '-'"
	}
	for _, segment := range strings.Split(p, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Sprintf("holds a %q segment", segment)
		}
		for _, prefix := range bundleEditPrefixes {
			if strings.HasPrefix(segment, prefix) {
				return fmt.Sprintf("has a segment beginning %q, which a directory write reserves", prefix)
			}
		}
	}
	return ""
}

func isBundlePathRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == '/', r == '.', r == '_', r == '-':
		return true
	}
	return false
}

// bundleDeletePrefix begins the name of a tar entry that deletes the path after it rather than writing a file.
const bundleDeletePrefix = "__DEL__"

// bundleEditPrefixes are the prefixes a directory write reserves at the start of every path segment.
var bundleEditPrefixes = []string{bundleDeletePrefix}
