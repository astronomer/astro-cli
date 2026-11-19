package localshared

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// A command's debug lines name its environment by key only: the engines add a
// project's secrets there.
func TestCommandLogNamesTheEnvironmentWithoutValues(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	lg := StartCommand(context.Background(), []string{"DOCKER_HOST=unix:///x", "API_TOKEN=s3cret"}, "docker", "compose", "up")
	if !lg.Debug() {
		t.Fatal("Debug() = false at debug level")
	}
	lg.End(errors.New("exit status 1"), []byte("no such service: api-server\n"))

	got := buf.String()
	for _, want := range []string{
		`msg=running cmd="docker compose up" env=DOCKER_HOST,API_TOKEN`,
		`msg=failed`, `err="exit status 1"`, `stderr="no such service: api-server"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("log lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "s3cret") || strings.Contains(got, "unix:///x") {
		t.Errorf("log holds an environment value:\n%s", got)
	}
}

// Nothing is logged, and no stderr worth capturing, below debug.
func TestCommandLogIsQuietBelowDebug(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	lg := StartCommand(context.Background(), nil, "docker", "ps")
	if lg.Debug() {
		t.Error("Debug() = true at warn level")
	}
	lg.End(nil, nil)
	if buf.Len() != 0 {
		t.Errorf("logged at warn level:\n%s", buf.String())
	}
}
