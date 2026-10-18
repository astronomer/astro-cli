package config

import (
	"os"
	"regexp"
)

// Setting names are dot-separated namespaces of snake_case words, as `astro
// config set` takes them: deploy.git_metadata, cosmos_boost.pre_deploy.
// api-server.port, removed, was the one exception; a new setting is held to
// it here rather than in review.
func (s *Suite) TestSettingNamesAreDottedSnakeCase() {
	name := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*(\.[a-z][a-z0-9]*(_[a-z0-9]+)*)*$`)
	for path := range CFGStrMap {
		s.Regexp(name, path)
	}
}

// A set value is stored as its default's type only when it reads back as
// what was typed.
func (s *Suite) TestTypedKeepsWhatWasTyped() {
	port := cfg{Default: "443"}
	s.Equal(8443, port.typed("8443"))
	s.Equal("0443", port.typed("0443"))
	s.Equal("+443", port.typed("+443"))
	s.Equal("https", port.typed("https"))

	flag := cfg{Default: "false"}
	s.Equal(true, flag.typed("true"))
	s.Equal("1", flag.typed("1"))

	zero := cfg{Default: "0"}
	s.Equal(1, zero.typed("1"), "a default of 0 is a number, not a bool")
}

func (s *Suite) TestNewCfg() {
	cfg := newCfg("foo", "bar")
	s.NotNil(cfg)
}

func (s *Suite) TestGetString() {
	initTestConfig()
	cfg := newCfg("foo", "0")
	cfg.SetHomeString("1")
	val := cfg.GetString()
	s.Equal("1", val)

	viperProject.SetConfigFile("test.yaml")
	defer os.Remove("test.yaml")
	cfg.SetProjectString("2")
	val = cfg.GetString()
	s.Equal("2", val)
}

func (s *Suite) TestGetInt() {
	initTestConfig()
	cfg := newCfg("foo", "0")
	cfg.SetHomeString("1")
	val := cfg.GetInt()
	s.Equal(1, val)

	viperProject.SetConfigFile("test.yaml")
	defer os.Remove("test.yaml")
	cfg.SetProjectString("2")
	val = cfg.GetInt()
	s.Equal(2, val)
}

func (s *Suite) TestGetBool() {
	initTestConfig()
	cfg := newCfg("foo", "false")
	cfg.SetHomeString("true")
	val := cfg.GetBool()
	s.Equal(true, val)

	viperProject.SetConfigFile("test.yaml")
	defer os.Remove("test.yaml")
	cfg.SetProjectString("false")
	val = cfg.GetBool()
	s.Equal(false, val)
}

// Before InitConfig has read a home config there is no context to find, and
// asking says so: a code path that builds the command tree before loading the
// config (a test run first by -shuffle) must not panic on the nil viper.
func (s *Suite) TestNoHomeConfigYetIsNoContext() {
	prev := viperHome
	viperHome = nil
	defer func() { viperHome = prev }()
	s.T().Setenv("ASTRO_DOMAIN", "")

	s.Equal("", CFG.Context.GetHomeString())
	_, err := GetCurrentContext()
	s.ErrorIs(err, ErrGetHomeString)
}
