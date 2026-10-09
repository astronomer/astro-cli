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
	got, err := IsProjectDir("")
	s.NoError(err)
	s.False(got, "an empty path names no directory, not the working one")
}

// The home directory is the same directory however it is reached: through a
// symlink, or spelled with a trailing separator.
func (s *Suite) TestIsHomeDir() {
	prev := HomePath
	defer func() { HomePath = prev }()
	home := s.T().TempDir()
	link := filepath.Join(s.T().TempDir(), "home")
	s.Require().NoError(os.Symlink(home, link))

	HomePath = link
	s.True(IsHomeDir(home))
	s.True(IsHomeDir(home + string(os.PathSeparator)))
	s.False(IsHomeDir(filepath.Join(home, "project")))

	HomePath = ""
	s.False(IsHomeDir(home), "no home directory known names none")
}

// The CLI's settings file is no project's .astro/config.yaml, wherever
// ASTRO_HOME puts it, and neither is the one in the home directory, where the
// settings live without it: under ASTRO_HOME, that one would otherwise put
// everything below ~ inside a 1.x project.
func (s *Suite) TestTheSettingsFileIsNoProject() {
	prevHome := HomePath
	defer func() { HomePath = prevHome }()
	write := func(dir string) {
		s.Require().NoError(os.MkdirAll(filepath.Join(dir, ConfigDir), 0o755))
		s.Require().NoError(os.WriteFile(filepath.Join(dir, ConfigDir, ConfigFileNameWithExt), []byte("context: cloud\n"), 0o600))
	}
	notProject := func(dir string) {
		s.T().Helper()
		isProject, err := IsProjectDir(dir)
		s.NoError(err)
		s.False(isProject, "the settings file made %s a project", dir)
		within, err := IsWithinProjectDir(filepath.Join(dir, "sub"))
		s.NoError(err)
		s.False(within)
		ok, err := IsAstroProject(dir)
		s.NoError(err)
		s.False(ok)
	}
	home, astroHome := s.T().TempDir(), s.T().TempDir()
	write(home)
	write(astroHome)
	HomePath = home

	s.Run("in the home directory", func() {
		s.withAstroHome("")
		initHome(afero.NewOsFs())
		notProject(home)
	})
	s.Run("under ASTRO_HOME", func() {
		s.withAstroHome(astroHome)
		initHome(afero.NewOsFs())
		notProject(astroHome)
		isProject, err := IsProjectDir(home)
		s.NoError(err)
		s.False(isProject, "with the settings under ASTRO_HOME, ~/.astro/config.yaml made ~ a project")
		within, err := IsWithinProjectDir(filepath.Join(home, "sub"))
		s.NoError(err)
		s.False(within)
	})
	s.Run("under ASTRO_HOME through a symlink", func() {
		link := filepath.Join(s.T().TempDir(), "astro-home")
		s.Require().NoError(os.Symlink(astroHome, link))
		s.withAstroHome(link)
		initHome(afero.NewOsFs())
		notProject(astroHome)
	})
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
	s.restoreConfigGlobals()
	initHome(fs)
	homeDir, err := fileutil.GetHomeDir()
	s.NoError(err)
	s.Equal(filepath.Join(homeDir, ".astro", "config.yaml"), viperHome.ConfigFileUsed())
}

func (s *Suite) TestInitHomeConfigOverride() {
	fs := afero.NewMemMapFs()
	// withAstroHome rather than a bare Setenv: unsetting the variable does
	// not undo what initHome did to HomeConfigFile and viperHome, and this
	// test used to leave both pointing at "test/.astro".
	s.withAstroHome("test")
	initHome(fs)
	s.Equal(filepath.Join("test", ".astro", "config.yaml"), viperHome.ConfigFileUsed())
}

func (s *Suite) TestInitProject() {
	fs := afero.NewMemMapFs()
	workingConfigPath := filepath.Join(WorkingPath, ConfigDir)
	workingConfigFile := filepath.Join(workingConfigPath, ConfigFileNameWithExt)
	fs.Create(workingConfigFile)
	initProject(fs)
	s.Contains(viperProject.ConfigFileUsed(), "config.yaml")
}

// TestSaveConfig_ConcurrentWritesProduceValidYAML stresses the file lock in
// saveConfig: 20 goroutines each write a distinct key/value pair to the same
// file. Without locking, viper.WriteConfigAs races interleave and can leave
// the YAML corrupted (unparseable) or truncated. With the flock in place,
// writes serialize and the final document is always valid YAML — we don't
// care which writer wins, only that the file is parseable and contains one
// of the expected values.
func (s *Suite) TestSaveConfig_ConcurrentWritesProduceValidYAML() {
	s.restoreConfigGlobals()
	configFs = afero.NewOsFs()
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
