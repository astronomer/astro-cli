package proxy

import (
	"net"
	"os"
	"strconv"
	"testing"
	"time"
)

// freePort is a loopback port nothing is listening on.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

// The test binary doubles as the daemon and as a bystander process, picked by
// environment variable, so these tests spawn real detached processes without
// needing an astro build.
const (
	helperServeEnv = "ASTRO_PROXY_TEST_SERVE_DIR"
	helperIdleEnv  = "ASTRO_PROXY_TEST_IDLE"
)

func TestMain(m *testing.M) {
	if dir := os.Getenv(helperServeEnv); dir != "" {
		// Spawned by Daemon.Start as <exe> serve --port <port>. The port is
		// parsed as the number it is rather than passed through as a raw
		// argument, which gosec traces into the files Serve writes.
		port, err := strconv.Atoi(os.Args[len(os.Args)-1])
		if err != nil {
			os.Exit(1)
		}
		d := &Daemon{Store: NewStore(dir)}
		if err := d.Serve(strconv.Itoa(port)); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if os.Getenv(helperIdleEnv) != "" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func readLog(d *Daemon) string {
	b, _ := os.ReadFile(d.LogPath())
	return string(b)
}
