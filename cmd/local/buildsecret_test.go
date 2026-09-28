package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/util"
)

// planRecorder is a runtime that records each plan a start hands it, and
// reports a running Airflow in mode for restart to find, or none when mode is
// empty.
type planRecorder struct {
	fakeRuntime
	plans *[]localrt.Plan
	mode  localrt.Mode
}

func (s planRecorder) Start(_ context.Context, p localrt.Plan, _ localrt.Callbacks) (localrt.Airflow, error) {
	*s.plans = append(*s.plans, p)
	return nil, localrt.ErrNotImplemented
}

func (s planRecorder) Attach(string) (localrt.Airflow, error) { return fakeAirflow{}, nil }

func (s planRecorder) ReadStatus(string) (localrt.Status, error) {
	if s.mode == "" {
		return localrt.Status{State: localrt.StateStopped}, nil
	}
	return localrt.Status{State: localrt.StateRunning, Mode: s.mode}, nil
}

const (
	declaredDockerfile = "dockerfile = 'Dockerfile'\n"
	matchingBase       = "astrocrpublic.azurecr.io/runtime:3.1-2"
)

func TestBuildSecretReachesTheDockerfileBuild(t *testing.T) {
	secrets := []string{"id=netrc,env=NETRC_CONTENT", "id=pip,src=/tmp/pip.conf"}
	flags := []string{"--build-secret", secrets[0], "--build-secret", secrets[1]}
	for _, tc := range []struct {
		name string
		args []string
		mode localrt.Mode
	}{
		{name: "start", args: []string{"local", "start", "--docker"}},
		{name: "root start", args: []string{"start", "--docker"}},
		{name: "restart", args: []string{"local", "restart"}, mode: localrt.ModeDocker},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, _ := testDeps(t)
			var plans []localrt.Plan
			d.Runtime = planRecorder{plans: &plans, mode: tc.mode}
			wiringProject(t, &d, declaredDockerfile, matchingBase)

			_ = execute(t, d, append(tc.args, flags...)...)
			if len(plans) != 1 {
				t.Fatalf("started %d times, want 1", len(plans))
			}
			if !slices.Equal(plans[0].BuildSecrets, secrets) {
				t.Errorf("BuildSecrets = %q, want %q", plans[0].BuildSecrets, secrets)
			}
		})
	}
}

func TestBuildSecretInputIsReadWhenTheFlagIsNotGiven(t *testing.T) {
	d, _ := testDeps(t)
	var plans []localrt.Plan
	d.Runtime = planRecorder{plans: &plans}
	wiringProject(t, &d, declaredDockerfile, matchingBase)
	t.Setenv(util.BuildSecretInputEnv, "id=netrc,env=NETRC_CONTENT\nid=pip,src=/tmp/pip.conf\n")

	_ = execute(t, d, "local", "start", "--docker")
	if len(plans) != 1 {
		t.Fatalf("started %d times, want 1", len(plans))
	}
	want := []string{"id=netrc,env=NETRC_CONTENT", "id=pip,src=/tmp/pip.conf"}
	if !slices.Equal(plans[0].BuildSecrets, want) {
		t.Errorf("BuildSecrets = %q, want %q", plans[0].BuildSecrets, want)
	}
}

func TestBuildSecretInputDoesNotRefuseAGeneratedBuild(t *testing.T) {
	d, _ := testDeps(t)
	var plans []localrt.Plan
	d.Runtime = planRecorder{plans: &plans}
	wiringProject(t, &d, "", "")
	t.Setenv(util.BuildSecretInputEnv, "id=netrc,env=NETRC_CONTENT")

	err := execute(t, d, "local", "start", "--docker")
	if errors.Is(err, util.ErrBuildSecretNeedsDockerfile) {
		t.Fatal("an exported BUILD_SECRET_INPUT refused a project with no Dockerfile")
	}
	if len(plans) != 1 {
		t.Errorf("started %d times, want 1", len(plans))
	}
}

func TestBuildSecretIsRefusedWithoutADockerfile(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		mode localrt.Mode
	}{
		{name: "start", args: []string{"local", "start", "--docker"}},
		{name: "start standalone", args: []string{"local", "start"}},
		{name: "restart", args: []string{"local", "restart"}, mode: localrt.ModeDocker},
		{name: "restart standalone", args: []string{"local", "restart"}, mode: localrt.ModeStandalone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, _ := testDeps(t)
			var plans []localrt.Plan
			d.Runtime = planRecorder{plans: &plans, mode: tc.mode}
			wiringProject(t, &d, "", "")

			err := execute(t, d, append(tc.args, "--build-secret", "id=netrc,env=NETRC_CONTENT")...)
			if !errors.Is(err, util.ErrBuildSecretNeedsDockerfile) {
				t.Fatalf("err = %v, want %v", err, util.ErrBuildSecretNeedsDockerfile)
			}
			if len(plans) != 0 {
				t.Error("the start reached the runtime")
			}
		})
	}
}

func TestBuildSecretInStandaloneWarnsAndStarts(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		mode localrt.Mode
	}{
		{name: "start", args: []string{"local", "start"}},
		{name: "restart", args: []string{"local", "restart"}, mode: localrt.ModeStandalone},
		{name: "restart with nothing running", args: []string{"local", "restart"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, stdout := testDeps(t)
			var plans []localrt.Plan
			d.Runtime = planRecorder{plans: &plans, mode: tc.mode}
			wiringProject(t, &d, declaredDockerfile, matchingBase)

			_ = execute(t, d, append(tc.args, "--build-secret", "id=netrc,env=NETRC_CONTENT")...)
			if !strings.Contains(stdout.String(), "warning: standalone mode builds no image, so --build-secret is not used") {
				t.Errorf("no warning: %q", stdout.String())
			}
			if len(plans) != 1 {
				t.Errorf("started %d times, want 1", len(plans))
			}
		})
	}
}

func TestPackageRefusesABuildSecretItCannotUse(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target string
		astro  string
		from   string
		want   string
	}{
		{name: "bucket target", target: "mwaa", astro: declaredDockerfile, from: matchingBase, want: "--build-secret has no effect with the mwaa target"},
		{name: "generated image", target: "astro", want: util.ErrBuildSecretNeedsDockerfile.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, _ := testDeps(t)
			wiringProject(t, &d, tc.astro, tc.from)

			err := execute(t, d, "package", tc.target, "--out-dir", t.TempDir(), "--build-secret", "id=netrc,env=NETRC_CONTENT")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

// A Docker-mode start warns, before it builds, about each secret the declared
// Dockerfile mounts that nothing supplies, and names the flag that would. The
// start goes ahead: a secret mount can be optional.
func TestStartWarnsAboutAnUnsuppliedSecretMount(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		warns bool
	}{
		{name: "none given", args: []string{"local", "start", "--docker"}, warns: true},
		{name: "given", args: []string{"local", "start", "--docker", "--build-secret", "id=netrc,env=NETRC_CONTENT"}},
		{name: "standalone builds nothing", args: []string{"local", "start"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(util.BuildSecretInputEnv, "")
			d, out := testDeps(t)
			var plans []localrt.Plan
			d.Runtime = planRecorder{plans: &plans}
			wiringProject(t, &d, declaredDockerfile, "")
			dir, _ := d.WorkingDir()
			dockerfile := "FROM " + matchingBase + "\nRUN --mount=type=secret,id=netrc,dst=/root/.netrc \\\n  pip install private\n"
			if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o600); err != nil {
				t.Fatal(err)
			}

			_ = execute(t, d, tc.args...)
			if len(plans) != 1 {
				t.Fatalf("started %d times, want 1", len(plans))
			}
			want := `warning: Dockerfile mounts build secret "netrc" (line 2) but none was given; pass --build-secret id=netrc,env=<VAR> or set BUILD_SECRET_INPUT`
			if got := strings.Contains(out.String(), want); got != tc.warns {
				t.Errorf("warned = %v, want %v; output:\n%s", got, tc.warns, out.String())
			}
		})
	}
}
