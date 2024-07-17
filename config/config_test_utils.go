package config

import (
	"io/fs"
	"os"
	"path/filepath"
)

func CreateTempProject() (dir string, cleanup func(), err error) {
	projectDir, err := os.MkdirTemp("", "")
	if err != nil {
		return "", nil, err
	}
	astroDirPerms := 0o755
	err = os.Mkdir(filepath.Join(projectDir, ".astro"), fs.FileMode(astroDirPerms))
	if err != nil {
		return "", nil, err
	}
	configFile, err := os.Create(filepath.Join(projectDir, ConfigDir, ConfigFileNameWithExt))
	if err != nil {
		return "", nil, err
	}
	return projectDir, func() {
		configFile.Close()
		os.RemoveAll(projectDir)
	}, nil
}
