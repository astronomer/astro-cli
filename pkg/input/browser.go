package input

import (
	"os"
)

// CanOpenBrowser reports whether this run may open the user's browser for
// them: when it may ask a question at all (CanAsk: a person at a terminal, no
// CI, no --non-interactive, no --output json) and is not over SSH, where a
// browser would open on the remote machine, not in front of the person.
//
// It is the one answer every command that hands off to a browser asks. A
// command that gets false still prints the URL, so the person (or the script
// reading it) can open it where they like.
func CanOpenBrowser() bool {
	if !CanAsk() {
		return false
	}
	return !overSSH(os.Getenv)
}

// overSSH reports whether getenv says this process runs in an SSH session.
func overSSH(getenv func(string) string) bool {
	return getenv("SSH_CONNECTION") != "" || getenv("SSH_TTY") != ""
}
