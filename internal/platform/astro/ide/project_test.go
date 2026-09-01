package ide

import (
	"os"
	"path/filepath"
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
	if err := createTarGzArchive(root, archivePath); err != nil {
		t.Fatalf("createTarGzArchive error: %v", err)
	}

	// Extract archive
	outDir := t.TempDir()
	if err := extractTarGzArchive(archivePath, outDir); err != nil {
		t.Fatalf("extractTarGzArchive error: %v", err)
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
