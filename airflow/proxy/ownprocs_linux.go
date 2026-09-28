package proxy

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// ownProcesses reads /proc, keeping the processes owned by this user.
func ownProcesses() []process {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	uid := os.Getuid()
	var out []process
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		dir := "/proc/" + e.Name()
		info, err := os.Stat(dir)
		if err != nil {
			continue
		}
		if st, ok := info.Sys().(*syscall.Stat_t); !ok || int(st.Uid) != uid {
			continue
		}
		args, err := os.ReadFile(filepath.Join(dir, "cmdline"))
		if err != nil {
			continue
		}
		env, err := os.ReadFile(filepath.Join(dir, "environ"))
		if err != nil {
			continue
		}
		out = append(out, process{pid: pid, args: splitNUL(args), env: splitNUL(env)})
	}
	return out
}

func splitNUL(b []byte) []string {
	var out []string
	for _, f := range bytes.Split(bytes.TrimRight(b, "\x00"), []byte{0}) {
		out = append(out, string(f))
	}
	return out
}
