package input

import "testing"

func TestOverSSH(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"local", nil, false},
		{"connection", map[string]string{"SSH_CONNECTION": "1.2.3.4 5 6.7.8.9 22"}, true},
		{"tty", map[string]string{"SSH_TTY": "/dev/pts/0"}, true},
		{"empty values", map[string]string{"SSH_CONNECTION": "", "SSH_TTY": ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := overSSH(func(k string) string { return tt.env[k] }); got != tt.want {
				t.Errorf("overSSH = %v, want %v", got, tt.want)
			}
		})
	}
}

// A run that may not ask may not open a browser either.
func TestCanOpenBrowserNeedsATerminal(t *testing.T) {
	terminals.Store(&[2]bool{false, true})
	t.Cleanup(func() { terminals.Store(nil) })
	if CanOpenBrowser() {
		t.Error("CanOpenBrowser with no terminal on stdin = true")
	}
	terminals.Store(&[2]bool{true, true})
	if !CanOpenBrowser() {
		t.Error("CanOpenBrowser with both terminals = false")
	}
}
