package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestConfig(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestIsProjectDir() {
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
		s.Run(tt.name, func() {
			got, err := IsProjectDir(tt.in)
			s.NoError(err)
			s.Equal(got, tt.out)
		})
	}
}

func (s *Suite) TestInitHomeDefaultCase() {
	fs := afero.NewMemMapFs()
	initHome(fs)
	homeDir, err := fileutil.GetHomeDir()
	s.NoError(err)
	s.Equal(filepath.Join(homeDir, ".astro", "config.yaml"), viperHome.ConfigFileUsed())
}

func (s *Suite) TestInitHomeConfigOverride() {
	fs := afero.NewMemMapFs()
	os.Setenv("ASTRO_HOME", "test")
	initHome(fs)
	s.Equal(filepath.Join("test", ".astro", "config.yaml"), viperHome.ConfigFileUsed())
	os.Unsetenv("ASTRO_HOME")
}

func (s *Suite) TestInitProject() {
	fs := afero.NewMemMapFs()
	workingConfigPath := filepath.Join(WorkingPath, ConfigDir)
	workingConfigFile := filepath.Join(workingConfigPath, ConfigFileNameWithExt)
	fs.Create(workingConfigFile)
	initProject(fs)
	s.Contains(viperProject.ConfigFileUsed(), "config.yaml")
}

func (s *Suite) TestProjectConfigExists() {
	initTestConfig()
	val := ProjectConfigExists()
	s.Equal(false, val)

	viperProject.SetConfigFile("test.yaml")
	defer os.Remove("test.yaml")
	val = ProjectConfigExists()
	s.Equal(true, val)
}

func (s *Suite) TestCreateConfig() {
	viperTest := viper.New()
	defer os.RemoveAll("./test")
	err := CreateConfig(viperTest, "./test", "test.yaml")
	s.NoError(err)
}

func (s *Suite) TestCreateProjectConfig() {
	viperProject = viper.New()
	defer os.RemoveAll("./test")
	CreateProjectConfig("./test")
	s.Equal("test/.astro/config.yaml", viperProject.ConfigFileUsed())
}
