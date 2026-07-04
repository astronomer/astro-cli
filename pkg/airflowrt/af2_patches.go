package airflowrt

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
)

// AF2DarwinShim is the macOS shim that replaces `airflow standalone` for AF2,
// avoiding gunicorn's fork which crashes on macOS.
//
//go:embed standalone_scripts/af2_darwin_shim.py
var AF2DarwinShim []byte

// AF2PickleFixPlugin fixes QueuedLocalWorker pickling under macOS spawn-mode multiprocessing.
//
//go:embed standalone_scripts/af2_pickle_fix_plugin.py
var AF2PickleFixPlugin []byte

// darwinForkSafetyPy / darwinForkSafetyPth are installed into the venv's
// site-packages to neutralize macOS fork-safety hazards at Python startup.
//
//go:embed standalone_scripts/fix_setproctitle.py
var darwinForkSafetyPy []byte

//go:embed standalone_scripts/fix_setproctitle.pth
var darwinForkSafetyPth []byte

// WriteDarwinForkSafetyPatch installs a .pth file into the venv's
// site-packages that neutralizes macOS fork-safety hazards at Python
// startup.  After os.fork() on macOS the Objective-C, CoreFoundation,
// and Network framework runtimes are in a corrupt state, causing forked
// children to spin at 100 % CPU on getaddrinfo, setproctitle, or any
// call that touches os_log / libdispatch.
//
// The patch does two things (only on macOS):
//
//  1. Removes os.fork from the Python namespace so that Airflow's
//     CAN_FORK = hasattr(os, "fork") evaluates to False.  This forces
//     standard_task_runner to use subprocess (_start_by_exec) instead of
//     the unsafe fork path (_start_by_fork).  subprocess.Popen uses
//     C-level fork+exec via _posixsubprocess, not os.fork, so it is
//     unaffected.
//
//  2. Replaces setproctitle.setproctitle with a no-op to prevent the
//     CoreFoundation CFBundleGetFunctionPointerForName spin in any
//     process that was still forked by other means.
//
// We use a .pth file (processed by Python's site module at startup)
// rather than sitecustomize.py because the venv's lib/pythonX.Y/
// directory is not on sys.path, whereas site-packages is.
func WriteDarwinForkSafetyPatch(venvPath string) error {
	libDir := filepath.Join(venvPath, "lib")
	entries, err := os.ReadDir(libDir)
	if err != nil {
		return err
	}

	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "python") {
			continue
		}

		sitePackages := filepath.Join(libDir, e.Name(), "site-packages")

		// Skip if already patched. We treat the .pth file as the sentinel —
		// both files are always written together, so its presence implies
		// _fix_setproctitle.py is also in place.
		if _, err := os.Stat(filepath.Join(sitePackages, "_fix_setproctitle.pth")); err == nil {
			continue
		}

		if err := os.WriteFile(filepath.Join(sitePackages, "_fix_setproctitle.py"), darwinForkSafetyPy, FilePermissions); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(sitePackages, "_fix_setproctitle.pth"), darwinForkSafetyPth, FilePermissions); err != nil {
			return err
		}
	}
	return nil
}
