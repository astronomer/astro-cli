package proxy

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	localhostSuffix = ".localhost"
	maxLabelLen     = 63
)

// nonAlphanumRe matches characters that are not lowercase alphanumeric or hyphens.
var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9-]+`)

// DeriveHostname converts a project directory path into a valid DNS hostname.
// It takes the base directory name, lowercases it, replaces non-alphanumeric
// characters with hyphens, trims leading/trailing hyphens, and appends ".localhost".
func DeriveHostname(projectDir string) (string, error) {
	name := filepath.Base(projectDir)
	name = strings.ToLower(name)
	name = nonAlphanumRe.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")

	if name == "" {
		return "", fmt.Errorf("could not derive a valid hostname from project directory %q", projectDir)
	}

	// Truncate to max DNS label length
	if len(name) > maxLabelLen {
		name = name[:maxLabelLen]
		name = strings.TrimRight(name, "-")
	}

	return name + localhostSuffix, nil
}
