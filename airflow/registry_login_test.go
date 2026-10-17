package airflow

import (
	"io"
	"path/filepath"
	"strings"
)

// assertStdinLogin checks a registry login ran as cliRegistryLogin runs it:
// the container runtime itself, not a shell, with exactly
// `login --username=<user> --password-stdin`, then `-- <registry>` when
// registry names one (nil: the test does not know which), and the password
// in none of the arguments, where other users could read it from the process
// list.
func (s *Suite) assertStdinLogin(cmd string, args []string, user string, registry *string, password string) {
	s.T().Helper()
	s.Contains([]string{"docker", "podman"}, filepath.Base(cmd), "the login ran %q, not the container runtime", cmd)
	s.Require().GreaterOrEqual(len(args), 3)
	s.Equal([]string{"login", "--username=" + user, "--password-stdin"}, args[:3])
	switch {
	case registry == nil:
	case *registry == "":
		s.Len(args, 3, "an empty registry is an argument")
	default:
		s.Equal([]string{"--", *registry}, args[3:])
	}
	// An empty password is in every string, and in none to leak.
	if secret := strings.TrimPrefix(password, prefix); secret != "" {
		for _, a := range args {
			s.NotContains(a, secret, "the password is in the login's arguments")
		}
	}
}

// One login for the three CLI login sites: the password on stdin, less
// "Bearer ", and options that a registry or username starting with "-"
// cannot become.
func (s *Suite) TestCLIRegistryLogin() {
	var gotCmd, gotStdin string
	var gotArgs []string
	// Suite.TearDownTest restores cmdExecWithStdin.
	cmdExecWithStdin = func(cmd, stdin string, _, _ io.Writer, args ...string) error {
		gotCmd, gotStdin, gotArgs = cmd, stdin, args
		return nil
	}

	cases := []struct {
		name, registry, user, password, wantStdin string
	}{
		{"a registry", "registry.example.com", "user", "secret", "secret"},
		{"no registry: the runtime's default", "", "user", "secret", "secret"},
		{"a Bearer token", "registry.example.com", "user", "Bearer tok", "tok"},
		{"a registry that looks like an option", "--password=leak", "user", "secret", "secret"},
		{"a username that looks like an option", "registry.example.com", "-x", "secret", "secret"},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			s.Require().NoError(cliRegistryLogin("docker", tc.registry, tc.user, tc.password, io.Discard, io.Discard))
			s.Equal(tc.wantStdin, gotStdin)
			registry := tc.registry
			s.assertStdinLogin(gotCmd, gotArgs, tc.user, &registry, tc.password)
		})
	}
}
