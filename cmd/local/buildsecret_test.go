package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/util"
)

// planRecorder is a runtime that records each plan a start hands it, and
// reports a running Airflow in mode for restart to find, or none when mode is
// empty. A start fails with startErr, or ErrNotImplemented when it is nil.
type planRecorder struct {
	fakeRuntime
	plans    *[]localrt.Plan
	mode     localrt.Mode
	startErr error
}

func (s planRecorder) Start(_ context.Context, p localrt.Plan, _ localrt.Callbacks) (localrt.Airflow, error) {
	*s.plans = append(*s.plans, p)
	if s.startErr != nil {
		return nil, s.startErr
	}
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
	t.Setenv("NETRC_CONTENT", "machine example.com")
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
	t.Setenv("NETRC_CONTENT", "machine example.com")
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
	t.Setenv(util.BuildSecretInputEnv, "id=pip,src=/tmp/pip.conf")

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

			err := execute(t, d, append(tc.args, "--build-secret", "id=pip,src=/tmp/pip.conf")...)
			if !errors.Is(err, util.ErrBuildSecretNeedsDockerfile) {
				t.Fatalf("err = %v, want %v", err, util.ErrBuildSecretNeedsDockerfile)
			}
			if len(plans) != 0 {
				t.Error("the start reached the runtime")
			}
		})
	}
}

// The runtime image mounts a netrc secret while it installs requirements, so a
// generated build takes one.
func TestNetrcBuildSecretReachesAGeneratedBuild(t *testing.T) {
	t.Setenv("NETRC_CONTENT", "machine example.com")
	d, _ := testDeps(t)
	var plans []localrt.Plan
	d.Runtime = planRecorder{plans: &plans}
	wiringProject(t, &d, "", "")

	_ = execute(t, d, "local", "start", "--docker", "--build-secret", "id=netrc,env=NETRC_CONTENT")
	if len(plans) != 1 {
		t.Fatalf("started %d times, want 1", len(plans))
	}
	if want := []string{"id=netrc,env=NETRC_CONTENT"}; !slices.Equal(plans[0].BuildSecrets, want) {
		t.Errorf("BuildSecrets = %q, want %q", plans[0].BuildSecrets, want)
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

			err := execute(t, d, "package", tc.target, "--out-dir", t.TempDir(), "--build-secret", "id=pip,src=/tmp/pip.conf")
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
	t.Setenv("NETRC_CONTENT", "machine example.com")
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

// The warning prints before the build, and a failed build's output pushes it
// out of sight, so the final error names the unsupplied secret again.
func TestFailedBuildNamesTheUnsuppliedSecretMount(t *testing.T) {
	t.Setenv("NETRC_CONTENT", "machine example.com")
	const hint = `Dockerfile mounts build secret "netrc", which was not given; pass --build-secret id=netrc,env=<VAR>`
	buildErr := fmt.Errorf("%w: exit status 1", imagebuild.ErrDockerfileBuild)
	for _, tc := range []struct {
		name     string
		args     []string
		mode     localrt.Mode
		startErr error
		hints    bool
	}{
		{name: "start", args: []string{"local", "start", "--docker"}, startErr: buildErr, hints: true},
		{name: "restart", args: []string{"local", "restart"}, mode: localrt.ModeDocker, startErr: buildErr, hints: true},
		{name: "secret given", args: []string{"local", "start", "--docker", "--build-secret", "id=netrc,env=NETRC_CONTENT"}, startErr: buildErr},
		{name: "not a build failure", args: []string{"local", "start", "--docker"}, startErr: errors.New("airflow never became healthy")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(util.BuildSecretInputEnv, "")
			d, _ := testDeps(t)
			var plans []localrt.Plan
			d.Runtime = planRecorder{plans: &plans, mode: tc.mode, startErr: tc.startErr}
			wiringProject(t, &d, declaredDockerfile, "")
			dir, _ := d.WorkingDir()
			dockerfile := "FROM " + matchingBase + "\nRUN --mount=type=secret,id=netrc,dst=/root/.netrc \\\n  pip install private\n"
			if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o600); err != nil {
				t.Fatal(err)
			}

			err := execute(t, d, tc.args...)
			if err == nil {
				t.Fatal("the start should fail")
			}
			if got := strings.Contains(err.Error(), hint); got != tc.hints {
				t.Errorf("hinted = %v, want %v; err: %v", got, tc.hints, err)
			}
		})
	}
}

const declaredBuildSecrets = declaredDockerfile + "build-secrets = ['id=netrc,env=NETRC_CONTENT']\n"

// A Docker-mode start builds with the manifest's build-secrets when no flag or
// BUILD_SECRET_INPUT gives any, and counts their ids as given. Standalone
// builds nothing and says nothing about them.
func TestManifestBuildSecretsReachTheDockerfileBuild(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		mode localrt.Mode
	}{
		{name: "start", args: []string{"local", "start", "--docker"}},
		{name: "restart", args: []string{"local", "restart"}, mode: localrt.ModeDocker},
		{name: "standalone", args: []string{"local", "start"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(util.BuildSecretInputEnv, "")
			t.Setenv("NETRC_CONTENT", "machine example.com")
			d, out := testDeps(t)
			var plans []localrt.Plan
			d.Runtime = planRecorder{plans: &plans, mode: tc.mode}
			wiringProject(t, &d, declaredBuildSecrets, "")
			dir, _ := d.WorkingDir()
			dockerfile := "FROM " + matchingBase + "\nRUN --mount=type=secret,id=netrc pip install private\n"
			if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o600); err != nil {
				t.Fatal(err)
			}

			_ = execute(t, d, tc.args...)
			if len(plans) != 1 {
				t.Fatalf("started %d times, want 1", len(plans))
			}
			if want := []string{"id=netrc,env=NETRC_CONTENT"}; !slices.Equal(plans[0].BuildSecrets, want) {
				t.Errorf("BuildSecrets = %q, want %q", plans[0].BuildSecrets, want)
			}
			if strings.Contains(out.String(), "secret") {
				t.Errorf("warned about a secret: %q", out.String())
			}
		})
	}
}

// A Docker-mode build whose secret names an unset variable stops before it
// builds, naming the secret and the variable. Standalone builds nothing, so
// it starts.
func TestStartRefusesABuildSecretWhoseVariableIsUnset(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		mode    localrt.Mode
		refuses bool
	}{
		{name: "start", args: []string{"local", "start", "--docker"}, refuses: true},
		{name: "restart", args: []string{"local", "restart"}, mode: localrt.ModeDocker, refuses: true},
		{name: "flag", args: []string{"local", "start", "--docker", "--build-secret", "id=pip,env=PIP_CONF"}, refuses: true},
		{name: "standalone", args: []string{"local", "start"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(util.BuildSecretInputEnv, "")
			t.Setenv("NETRC_CONTENT", "")
			t.Setenv("PIP_CONF", "")
			d, _ := testDeps(t)
			var plans []localrt.Plan
			d.Runtime = planRecorder{plans: &plans, mode: tc.mode}
			wiringProject(t, &d, declaredBuildSecrets, matchingBase)

			err := execute(t, d, tc.args...)
			refused := err != nil && strings.Contains(err.Error(), "reads the environment variable")
			if refused != tc.refuses {
				t.Fatalf("refused = %v, want %v; err: %v", refused, tc.refuses, err)
			}
			if tc.refuses && len(plans) != 0 {
				t.Error("the start reached the runtime")
			}
		})
	}
}

// astro package astro reads the manifest's build-secrets too, and refuses one
// whose variable is unset before it looks for Docker.
func TestPackageReadsManifestBuildSecrets(t *testing.T) {
	t.Setenv(util.BuildSecretInputEnv, "")
	t.Setenv("NETRC_CONTENT", "")
	d, _ := testDeps(t)
	wiringProject(t, &d, declaredBuildSecrets, matchingBase)

	err := execute(t, d, "package", "astro")
	if err == nil || !strings.Contains(err.Error(), `build secret "netrc" reads the environment variable NETRC_CONTENT`) {
		t.Fatalf("err = %v, want the unset variable named", err)
	}
}
