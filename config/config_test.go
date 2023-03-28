package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestIsProjectDir(t *testing.T) {
	homeDir, _ := fileutil.GetHomeDir()
	tests := []struct {
		name string
		in   string
		out  bool
	}{
		{"False", "", false},
		{"HomePath False", homeDir, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := IsProjectDir(tt.in)
			assert.NoError(t, err)
			assert.Equal(t, s, tt.out)
		})
	}
}

func TestInitHomeDefaultCase(t *testing.T) {
	fs := afero.NewMemMapFs()
	initHome(fs)
	homeDir, err := fileutil.GetHomeDir()
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(homeDir, ".astro", "config.yaml"), viperHome.ConfigFileUsed())
}

func TestInitHomeConfigOverride(t *testing.T) {
	fs := afero.NewMemMapFs()
	os.Setenv("ASTRO_HOME", "test")
	initHome(fs)
	assert.Equal(t, filepath.Join("test", ".astro", "config.yaml"), viperHome.ConfigFileUsed())
	os.Unsetenv("ASTRO_HOME")
}

func TestInitProject(t *testing.T) {
	fs := afero.NewMemMapFs()
	workingConfigPath := filepath.Join(WorkingPath, ConfigDir)
	workingConfigFile := filepath.Join(workingConfigPath, ConfigFileNameWithExt)
	fs.Create(workingConfigFile)
	initProject(fs)
	assert.Contains(t, viperProject.ConfigFileUsed(), "config.yaml")
}

func TestProjectConfigExists(t *testing.T) {
	initTestConfig()
	val := ProjectConfigExists()
	assert.Equal(t, false, val)

	viperProject.SetConfigFile("test.yaml")
	defer os.Remove("test.yaml")
	val = ProjectConfigExists()
	assert.Equal(t, true, val)
}

func TestCreateConfig(t *testing.T) {
	viperTest := viper.New()
	defer os.RemoveAll("./test")
	err := CreateConfig(viperTest, "./test", "test.yaml")
	assert.NoError(t, err)
}

func TestCreateProjectConfig(t *testing.T) {
	viperProject = viper.New()
	defer os.RemoveAll("./test")
	CreateProjectConfig("./test")
	assert.Equal(t, "test/.astro/config.yaml", viperProject.ConfigFileUsed())
}
