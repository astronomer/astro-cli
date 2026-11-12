package input

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The interaction mode is resolved in one place: json, a truthy CI, or
// --non-interactive refuse every question; --non-interactive=false asks again
// despite CI, but never under json; anything else may ask.
func TestRefusal(t *testing.T) {
	for _, tt := range []struct {
		name string
		ci   string
		env  map[string]string
		json bool
		args []string
		want string // a substring of the reason; "" means the run may ask
	}{
		{name: "nothing set may ask"},
		{name: "CI=true", ci: "true", want: "CI"},
		{name: "CI=1", ci: "1", want: "CI"},
		{name: "CI=TRUE", ci: "TRUE", want: "CI"},
		{name: "CI=yes", ci: "yes", want: "CI"},
		{name: "CI=false", ci: "false"},
		{name: "CI with surrounding spaces is trimmed", ci: " true ", want: "CI"},
		{name: "CI=false with surrounding spaces is trimmed", ci: " false "},
		{name: "CI=0 with surrounding spaces is trimmed", ci: " 0 "},
		{name: "JENKINS_URL set", env: map[string]string{"JENKINS_URL": "https://ci.example.com/"}, want: "Jenkins"},
		{name: "HUDSON_URL set", env: map[string]string{"HUDSON_URL": "https://ci.example.com/"}, want: "Jenkins"},
		{name: "JENKINS_URL empty", env: map[string]string{"JENKINS_URL": ""}},
		{name: "TF_BUILD=True", env: map[string]string{"TF_BUILD": "True"}, want: "TF_BUILD"},
		{name: "TF_BUILD=false", env: map[string]string{"TF_BUILD": "false"}},
		{name: "JENKINS_URL with --non-interactive=false asks", env: map[string]string{"JENKINS_URL": "x"}, args: []string{"--non-interactive=false"}},
		{name: "TF_BUILD with --non-interactive=false asks", env: map[string]string{"TF_BUILD": "True"}, args: []string{"--non-interactive=false"}},
		{name: "JENKINS_URL under json is json", env: map[string]string{"JENKINS_URL": "x"}, json: true, want: "--output json"},
		{name: "CI=False", ci: "False"},
		{name: "CI=0", ci: "0"},
		{name: "CI empty", ci: ""},
		{name: "--non-interactive", args: []string{"--non-interactive"}, want: "--non-interactive"},
		{name: "--non-interactive=true", args: []string{"--non-interactive=true"}, want: "--non-interactive"},
		{name: "--non-interactive=false", args: []string{"--non-interactive=false"}},
		{name: "--non-interactive=false with CI asks again", ci: "true", args: []string{"--non-interactive=false"}},
		{name: "--non-interactive with CI names the flag", ci: "true", args: []string{"--non-interactive"}, want: "--non-interactive"},
		{name: "json", json: true, want: "--output json"},
		{name: "json beats --non-interactive=false", json: true, args: []string{"--non-interactive=false"}, want: "--output json"},
		{name: "json with CI=false", json: true, ci: "false", want: "--output json"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CI", tt.ci)
			// Whatever CI system this test runs on must not decide it.
			for _, k := range []string{"JENKINS_URL", "HUDSON_URL", "TF_BUILD"} {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
			AddFlag(fs)
			require.NoError(t, fs.Parse(tt.args))

			got := Refusal(tt.json, fs.Lookup(NonInteractiveFlag))
			if tt.want == "" {
				assert.Empty(t, got)
				return
			}
			assert.Contains(t, got, tt.want)
		})
	}

	t.Run("a command with no such flag still reads CI", func(t *testing.T) {
		for _, k := range []string{"JENKINS_URL", "HUDSON_URL", "TF_BUILD"} {
			t.Setenv(k, "")
		}
		t.Setenv("CI", "true")
		assert.Contains(t, Refusal(false, nil), "CI")
		t.Setenv("CI", "")
		assert.Empty(t, Refusal(false, nil))
	})
}

// CanAsk follows the guard the root installs.
func TestCanAskFollowsTheGuard(t *testing.T) {
	assert.True(t, CanAsk(), "no guard, may ask")
	restore := SetGuard(func() string { return "with CI set it cannot" })
	assert.False(t, CanAsk())
	restore()
	assert.True(t, CanAsk())
}
