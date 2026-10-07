package ide

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	full := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(full), DefaultDirPerm); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

func mustMkdir(t *testing.T, root, relPath string) {
	t.Helper()
	full := filepath.Join(root, relPath)
	if err := os.MkdirAll(full, DefaultDirPerm); err != nil {
		t.Fatalf("mkdir %s: %v", full, err)
	}
}

func TestCreateAndExtractArchive_RespectsGitignoreAndAllowlist(t *testing.T) {
	root := t.TempDir()
	// .gitignore patterns: ignore logs, .venv, and all dotfiles by default
	writeFile(t, root, ".gitignore", "*.log\n.venv/\n.*\n")
	mustMkdir(t, root, "include/sql")
	writeFile(t, root, "include/sql/index.sql", "select 1\n")
	mustMkdir(t, root, ".venv/bin")
	writeFile(t, root, ".venv/bin/activate", "#!/bin/sh\n")
	mustMkdir(t, root, ".astro")
	writeFile(t, root, ".astro/config.yaml", "name: test\n")
	writeFile(t, root, ".dockerignore", "*.tmp\n")
	mustMkdir(t, root, ".git")
	writeFile(t, root, ".git/config", "[core]\n\trepositoryformatversion = 0\n")
	mustMkdir(t, root, "dags")
	writeFile(t, root, "dags/.airflowignore", "*.log\n")
	writeFile(t, root, "dags/main.py", "print('hello')\n")
	writeFile(t, root, "Dockerfile", "FROM astrocrpublic.azurecr.io/runtime:3.0-7\n")
	writeFile(t, root, "requirements.txt", "pytest-html\n")
	writeFile(t, root, "packages.txt", "curl\n")
	writeFile(t, root, "debug.log", "ignore me\n")
	writeFile(t, root, ".test", "test\n")

	// Create archive
	archiveDir := t.TempDir()
	archivePath := filepath.Join(archiveDir, "project.tar.gz")
	created, err := createTarGzArchive(root, archivePath, io.Discard)
	if err != nil {
		t.Fatalf("createTarGzArchive error: %v", err)
	}

	// Extract archive
	outDir := t.TempDir()
	extracted, err := extractAt(t.Context(), archivePath, outDir)
	if err != nil {
		t.Fatalf("extractTarGzArchive error: %v", err)
	}
	// The nine files kept, below: what was archived is what was written.
	want := archiveStats{files: 9, bytes: int64(len("select 1\n") + len("name: test\n") + len("*.tmp\n") + len("*.log\n") +
		len("print('hello')\n") + len("FROM astrocrpublic.azurecr.io/runtime:3.0-7\n") + len("pytest-html\n") + len("curl\n") + len("*.log\n.venv/\n.*\n"))}
	if created != want || extracted != want {
		t.Errorf("archived %+v and extracted %+v, want %+v", created, extracted, want)
	}

	// Expect included
	if _, err := os.Stat(filepath.Join(outDir, "dags", "main.py")); err != nil {
		t.Errorf("expected main.py to be extracted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "Dockerfile")); err != nil {
		t.Errorf("expected Dockerfile to be extracted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, ".gitignore")); err != nil {
		t.Errorf("expected .gitignore to be extracted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, ".dockerignore")); err != nil {
		t.Errorf("expected .dockerignore to be extracted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, ".astro", "config.yaml")); err != nil {
		t.Errorf("expected .astro/config.yaml to be extracted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "dags", ".airflowignore")); err != nil {
		t.Errorf("expected dags/.airflowignore to be extracted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "requirements.txt")); err != nil {
		t.Errorf("expected requirements.txt to be extracted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "packages.txt")); err != nil {
		t.Errorf("expected packages.txt to be extracted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "include", "sql", "index.sql")); err != nil {
		t.Errorf("expected include/sql/index.sql to be extracted: %v", err)
	}

	// Expect excluded (gitignore or explicit rules)
	if _, err := os.Stat(filepath.Join(outDir, "debug.log")); !os.IsNotExist(err) {
		t.Errorf("expected debug.log to be excluded, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, ".venv", "bin", "activate")); !os.IsNotExist(err) {
		t.Errorf("expected .venv/bin/activate to be excluded, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, ".git", "config")); !os.IsNotExist(err) {
		t.Errorf("expected .git/config to be excluded, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, ".test")); !os.IsNotExist(err) {
		t.Errorf("expected .test to be excluded, got err=%v", err)
	}
}

// An import over a file that is longer than the one imported leaves the
// imported file, not the imported bytes followed by the old file's tail.
func TestExtractReplacesALongerFile(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "dags/a.py", "new\n")
	archivePath := filepath.Join(t.TempDir(), "project.tar.gz")
	if _, err := createTarGzArchive(src, archivePath, io.Discard); err != nil {
		t.Fatalf("createTarGzArchive error: %v", err)
	}

	dst := t.TempDir()
	writeFile(t, dst, "dags/a.py", "a much longer old file\n")
	if _, err := extractAt(t.Context(), archivePath, dst); err != nil {
		t.Fatalf("extractTarGzArchive error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "dags", "a.py"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new\n" {
		t.Errorf("dags/a.py holds %q, want %q", got, "new\n")
	}
}

func TestOpenInBrowser(t *testing.T) {
	prev := openURL
	t.Cleanup(func() { openURL = prev })
	var opened []string
	openURL = func(url string) error {
		opened = append(opened, url)
		return errors.New("no browser")
	}

	var out strings.Builder
	OpenInBrowser("", &out)
	if len(opened) != 0 || out.Len() != 0 {
		t.Errorf("a project with no URL opened %v and said %q", opened, out.String())
	}

	OpenInBrowser("https://ide.example/p", &out)
	if len(opened) != 1 || opened[0] != "https://ide.example/p" {
		t.Errorf("opened %v", opened)
	}
	if want := "Unable to open the Astro IDE project URL, please visit the following link: https://ide.example/p\n"; out.String() != want {
		t.Errorf("said %q, want %q", out.String(), want)
	}
}
