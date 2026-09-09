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

func (s *Suite) TestDefaultsAreNotWrittenToDisk() {
	fs := afero.NewMemMapFs()
	initHome(fs)
	initProject(fs)

	// A freshly created config holds nothing the user did not ask for, so a later
	// change to any default reaches them.
	contents, err := afero.ReadFile(fs, HomeConfigFile)
	s.NoError(err)
	s.NotContains(string(contents), "postgres")
	s.Equal(PostgresTagDefault, CFG.PostgresTag.GetString())

	// Writing one setting must not drag every other default onto disk with it.
	s.NoError(CFG.PageSize.SetHomeString("30"))
	contents, err = afero.ReadFile(fs, HomeConfigFile)
	s.NoError(err)
	s.NotContains(string(contents), "postgres")
	s.Equal("30", CFG.PageSize.GetString())
	s.Equal(PostgresTagDefault, CFG.PostgresTag.GetString())
}

// writeHomeConfig lays down a config file an older CLI would have left behind, then
// starts up against it. The file has to exist before initHome runs: creating one is what
// stamps it, and a stamped file is never migrated.
func (s *Suite) writeHomeConfig(fs afero.Fs, content string) {
	initHome(fs)
	s.NoError(afero.WriteFile(fs, HomeConfigFile, []byte(content), 0o600))
	initHome(fs)
	initProject(fs)
}

const legacyHomeConfig = `context: astronomer.io
contexts:
    astronomer_io:
        domain: astronomer.io
        token: Bearer sometoken
        workspace: someworkspace
duplicate_volumes: "true"
page_size: "20"
postgres:
    port: "5432"
    tag: "12.6"
    user: postgres
webserver:
    port: "8080"
`

func (s *Suite) TestMigrateHomeConfig() {
	s.Run("drops entries matching a default the CLI has shipped", func() {
		fs := afero.NewMemMapFs()
		s.writeHomeConfig(fs, legacyHomeConfig)

		migrateHomeConfig(fs)

		// 12.6 was the default when the file was written, so it was never a choice.
		s.False(viperHome.IsSet(CFG.PostgresTag.Path))
		s.Equal(PostgresTagDefault, CFG.PostgresTag.GetString())
		// Values still matching the current default go too, so a future change to any
		// of them reaches this user.
		s.False(viperHome.IsSet(CFG.PageSize.Path))
		s.False(viperHome.IsSet(CFG.WebserverPort.Path))
		s.False(viperHome.IsSet(CFG.DuplicateImageVolumes.Path))
		s.Equal(20, CFG.PageSize.GetInt())
		s.True(CFG.DuplicateImageVolumes.GetBool())
	})

	s.Run("keeps everything the user chose", func() {
		fs := afero.NewMemMapFs()
		s.writeHomeConfig(fs, "page_size: \"50\"\npostgres:\n    tag: \"13.4\"\n")

		migrateHomeConfig(fs)

		s.Equal("13.4", CFG.PostgresTag.GetString())
		s.Equal(50, CFG.PageSize.GetInt())
	})

	s.Run("leaves contexts and tokens untouched", func() {
		fs := afero.NewMemMapFs()
		s.writeHomeConfig(fs, legacyHomeConfig)

		migrateHomeConfig(fs)

		ctx, err := GetCurrentContext()
		s.NoError(err)
		s.Equal("Bearer sometoken", ctx.Token)
		s.Equal("someworkspace", ctx.Workspace)
		s.Equal("astronomer.io", ctx.Domain)
	})

	s.Run("stamps the file and never runs again", func() {
		fs := afero.NewMemMapFs()
		s.writeHomeConfig(fs, legacyHomeConfig)

		migrateHomeConfig(fs)
		s.Equal(currentConfigVersion, viperHome.GetInt(configVersionKey))

		// Once stamped, a value equal to a shipped default is the user's own.
		s.NoError(CFG.PostgresTag.SetHomeString("12.6"))
		migrateHomeConfig(fs)
		s.Equal("12.6", CFG.PostgresTag.GetString())
	})

	s.Run("a file this version wrote is never scanned", func() {
		fs := afero.NewMemMapFs()
		initHome(fs)
		initProject(fs)

		contents, err := afero.ReadFile(fs, HomeConfigFile)
		s.NoError(err)
		s.Contains(string(contents), configVersionKey)
		s.Equal(currentConfigVersion, viperHome.GetInt(configVersionKey))
	})

	s.Run("startup runs the migration", func() {
		// Calling migrateHomeConfig directly proves the migration works; this proves
		// the CLI actually performs it.
		fs := afero.NewMemMapFs()
		initHome(fs)
		s.NoError(afero.WriteFile(fs, HomeConfigFile, []byte(legacyHomeConfig), 0o600))

		InitConfig(fs)

		s.Equal(currentConfigVersion, viperHome.GetInt(configVersionKey))
		s.Equal(PostgresTagDefault, CFG.PostgresTag.GetString())
	})

	s.Run("an unwritable config is not fatal and prints nothing", func() {
		fs := afero.NewReadOnlyFs(afero.NewMemMapFs())
		initHome(fs)
		initProject(fs)

		s.NotPanics(func() { migrateHomeConfig(fs) })
	})
}

func (s *Suite) TestDeleteNested() {
	settings := map[string]any{
		"postgres":  map[string]any{"tag": "12.6", "user": "postgres"},
		"page_size": "20",
	}

	deleteNested(settings, "postgres.tag")
	s.Equal(map[string]any{"postgres": map[string]any{"user": "postgres"}, "page_size": "20"}, settings)

	// A parent left empty goes with its last child.
	deleteNested(settings, "postgres.user")
	s.Equal(map[string]any{"page_size": "20"}, settings)

	// Absent keys and non-map parents are left alone.
	deleteNested(settings, "nothing.here")
	deleteNested(settings, "page_size.nested")
	s.Equal(map[string]any{"page_size": "20"}, settings)
}
