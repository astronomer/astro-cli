// Package localenv is the local env-values feature: the project's dotenv file
// that holds its Airflow values, the provider chain that resolves them, and
// the set/get/list/delete operations behind `astro local env`.
//
// It writes plain files — no keyring, no encryption (docs/v2-secrets.md). The
// encrypted tiers are internal/vaultenv's, and they are passed INTO the chain
// this package assembles rather than built here, which is what keeps that true:
//
//	project .env > shell env > project vault > global vault
//
// cmd renders; nothing here prints or exits.
package localenv

import (
	"path/filepath"
)

// EnvFileName is the project-scoped dotenv file, at the project root.
const EnvFileName = ".env"

// filePerm is the owner-only mode every env file is created with, so another
// OS user on a shared machine cannot read it.
const filePerm = 0o600

// ProjectEnvPath is the project-scoped file: <project>/.env.
func ProjectEnvPath(projectDir string) string {
	return filepath.Join(projectDir, EnvFileName)
}
