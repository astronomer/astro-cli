package config

import (
	"os"
)

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
