package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/afero"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/pkg/fileutil"
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

func (s *Suite) TestIsWithinProjectDir() {
	projectDir, cleanupProjectDir, err := CreateTempProject()
	s.NoError(err)
	defer cleanupProjectDir()

	anotherDir, err := os.MkdirTemp("", "")
	s.NoError(err)

	tests := []struct {
		name string
		in   string
		out  bool
	}{
		{"not in", anotherDir, false},
		{"at", projectDir, true},
		{"just in", filepath.Join(projectDir, "test"), true},
		{"deep in", filepath.Join(projectDir, "test", "test", "test"), true},
		{"root", string(os.PathSeparator), false},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := IsWithinProjectDir(tt.in)
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
	err := CreateConfig(viperTest, afero.NewOsFs(), "./test", "test.yaml")
	s.NoError(err)
}

func (s *Suite) TestCreateProjectConfig() {
	viperProject = viper.New()
	defer os.RemoveAll("./test")
	CreateProjectConfig("./test")
	s.Equal("test/.astro/config.yaml", viperProject.ConfigFileUsed())
}

// TestSaveConfig_ConcurrentWritesProduceValidYAML stresses the file lock in
// saveConfig: 20 goroutines each write a distinct key/value pair to the same
// file. Without locking, viper.WriteConfigAs races interleave and can leave
// the YAML corrupted (unparseable) or truncated. With the flock in place,
// writes serialize and the final document is always valid YAML — we don't
// care which writer wins, only that the file is parseable and contains one
// of the expected values.
func (s *Suite) TestSaveConfig_ConcurrentWritesProduceValidYAML() {
	dir := s.T().TempDir()
	file := filepath.Join(dir, "config.yaml")

	const writers = 20
	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func(i int) {
			defer wg.Done()
			v := viper.New()
			v.SetConfigType("yaml")
			v.Set("writer", fmt.Sprintf("w%d", i))
			s.NoError(saveConfig(v, file))
		}(i)
	}
	wg.Wait()

	// File must exist and parse cleanly as YAML every time.
	raw, err := os.ReadFile(file)
	s.Require().NoError(err)
	var parsed map[string]any
	s.Require().NoError(yaml.Unmarshal(raw, &parsed), "file contents: %q", raw)

	// Some writer's value must have won. No partial keys, no corruption.
	winner, ok := parsed["writer"].(string)
	s.Require().True(ok, "writer key missing or not a string: %v", parsed)
	s.Regexp(`^w\d+$`, winner)
}
