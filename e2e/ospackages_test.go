//go:build e2e && !windows

package e2e

import (
	"os/exec"
	"strings"
	"testing"
)

// The two OS packages this case turns on. Both are small, both are in Debian
// main, and neither is anything an Airflow image would carry for its own sake.
//
// declaredPackage is asked for in the manifest. controlPackage is not, and is
// its twin in every other respect — same size, same obscurity, same repo. It
// is there because "the package is installed" is only evidence if the image
// would not have had it anyway: a base image that happened to carry tree would
// make this case pass while proving nothing, which is exactly the shape of
// assertion that has slipped through here before.
const (
	declaredPackage = "tree"
	controlPackage  = "sl"
)

// Docker mode installs the OS packages the manifest declares.
//
// T7.4's first claim, and the last part of the manifest's build contract with
// no test behind it. The plumbing reads as though it works — the manifest's
// `packages` reach the build request, imagebuild writes them to packages.txt,
// and the runtime image's ONBUILD trigger apt-installs that file — but no case
// had ever asked the running container whether any of it happened. A dropped
// field anywhere along that path produces an image that starts perfectly and
// is missing what the project said it needed, which is only discovered by the
// DAG that imports psycopg2.
func TestDockerModeInstallsDeclaredOSPackages(t *testing.T) {
	tier(t, 3)
	p := dockerProject(t, "ospkgs")
	needsDocker(t, p)

	declarePackages(t, p, declaredPackage)

	p.runSlow("local", "start", "--docker").requireSuccess()
	if st := p.status(); st.State != "running" {
		t.Fatalf("state = %q, want running", st.State)
	}

	// One invocation answering both questions, and exiting 0 either way: `astro
	// local run` surfaces the container command's failure as its own, which
	// would make "not installed" indistinguishable from "the command could not
	// be run at all".
	script := "dpkg -s " + declaredPackage + " >/dev/null 2>&1 && echo DECLARED-YES || echo DECLARED-NO; " +
		"dpkg -s " + controlPackage + " >/dev/null 2>&1 && echo CONTROL-YES || echo CONTROL-NO"
	got := p.runSlow("local", "run", "--", "sh", "-c", script).requireSuccess()

	// Both markers before either verdict. Absent output would otherwise read as
	// "no CONTROL-YES" and pass the guard below, then fail the declared check
	// and blame imagebuild for what was really the harness losing the
	// container's stdout.
	declared, ok := marker(t, got.Stdout, "DECLARED")
	if !ok {
		t.Fatalf("the container answered with neither DECLARED marker, so nothing here can be read\n%s", got.output())
	}
	control, ok := marker(t, got.Stdout, "CONTROL")
	if !ok {
		t.Fatalf("the container answered with neither CONTROL marker, so nothing here can be read\n%s", got.output())
	}

	// The control first: if the image carries it uninvited, the other half of
	// this case means nothing and should not be reported as a pass.
	if control {
		t.Fatalf("the base image already has %s, so finding %s proves nothing about the manifest\n%s",
			controlPackage, declaredPackage, got.output())
	}
	if !declared {
		t.Errorf("the manifest declared %s and the running container does not have it\n%s",
			declaredPackage, got.output())
	}
}

// The base image does not already carry the declared package.
//
// The control package rules out the base carrying that CLASS of tooling; it
// says nothing about this one. So the base is asked directly, which is the only
// thing that keeps the case above honest if a future Astro Runtime starts
// shipping tree — it would go permanently green while proving nothing about
// whether the manifest's packages ever reached packages.txt.
//
// Cheap here: the image this pulls is the one the case above has already
// pulled.
func TestTheBaseImageDoesNotAlreadyHaveTheDeclaredPackage(t *testing.T) {
	tier(t, 3)
	p := dockerProject(t, "ospkgbase")
	needsDocker(t, p)

	base := runtimeImageFor(t, p)
	out, err := exec.CommandContext(t.Context(), "docker", "run", "--rm", "--entrypoint", "sh", base,
		"-c", "dpkg -s "+declaredPackage+" >/dev/null 2>&1 && echo BASE-YES || echo BASE-NO").CombinedOutput()
	if err != nil {
		t.Fatalf("asking the base image about %s: %v\n%s", declaredPackage, err, out)
	}
	if !strings.Contains(string(out), "BASE-NO") {
		t.Errorf("the base image already carries %s, so the case that declares it proves nothing:\n%s",
			declaredPackage, out)
	}
}

// marker reads one YES/NO pair out of the script's output, reporting whether
// either was there at all.
func marker(t *testing.T, out, name string) (yes, found bool) {
	t.Helper()
	switch {
	case strings.Contains(out, name+"-YES"):
		return true, true
	case strings.Contains(out, name+"-NO"):
		return false, true
	default:
		return false, false
	}
}

// declarePackages adds `packages = [...]` under [tool.astro].
func declarePackages(t *testing.T, p *project, names ...string) {
	t.Helper()
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = `"` + n + `"`
	}
	addAstroKey(t, p, "packages = ["+strings.Join(quoted, ", ")+"]")
}
