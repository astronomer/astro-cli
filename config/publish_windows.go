package config

import (
	iofs "io/fs"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
)

// publishConfig publishes data as path with mode perm. Windows has no POSIX
// owner for a replacing file to lose, so there is nothing to restore; see the
// Unix version for why that one does.
func publishConfig(path string, data []byte, perm iofs.FileMode, _ iofs.FileInfo) error {
	return fsatomic.WriteFile(path, data, perm)
}
