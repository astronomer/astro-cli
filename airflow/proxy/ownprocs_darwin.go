package proxy

import (
	"bytes"
	"encoding/binary"
	"os"

	"golang.org/x/sys/unix"
)

// ownProcesses lists this user's processes through sysctl, and reads each
// one's arguments and environment from kern.procargs2, the source ps -E uses.
func ownProcesses() []process {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.uid", os.Getuid())
	if err != nil {
		return nil
	}
	var out []process
	for i := range procs {
		pid := int(procs[i].Proc.P_pid)
		raw, err := unix.SysctlRaw("kern.procargs2", pid)
		if err != nil {
			continue
		}
		if args, env, ok := parseProcArgs(raw); ok {
			out = append(out, process{pid: pid, args: args, env: env})
		}
	}
	return out
}

// parseProcArgs splits a kern.procargs2 buffer: argc as a 32-bit integer, the
// executable path, NUL padding, argc arguments, then the environment up to the
// first empty string.
func parseProcArgs(raw []byte) (args, env []string, ok bool) {
	const argcSize = 4
	if len(raw) < argcSize {
		return nil, nil, false
	}
	argc := int(binary.LittleEndian.Uint32(raw[:argcSize]))
	rest := raw[argcSize:]
	end := bytes.IndexByte(rest, 0)
	if end < 0 {
		return nil, nil, false
	}
	fields := bytes.Split(bytes.TrimLeft(rest[end:], "\x00"), []byte{0})
	if len(fields) < argc {
		return nil, nil, false
	}
	for _, f := range fields[:argc] {
		args = append(args, string(f))
	}
	for _, f := range fields[argc:] {
		if len(f) == 0 {
			break
		}
		env = append(env, string(f))
	}
	return args, env, true
}
