// Package localenv is the local env-values feature: the dotenv files that
// hold a project's and a user's Airflow values, the provider chain that
// resolves them, and the set/get/list/delete operations behind
// `astro local env`.
//
// It writes plain files — no keyring, no encryption (docs/v2-secrets.md). The
// encrypted tiers are internal/vaultenv's, and they are passed INTO the chain
// this package assembles rather than built here, which is what keeps that true:
//
//	shell env > project .env > project vault > global vault > global ~/.astro/env
//
// cmd renders; nothing here prints or exits.
package localenv

import (
	"os"
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

// GlobalEnvPath is the machine-wide file: ~/.astro/env, or
// $ASTRO_HOME/.astro/env when that is set.
//
// ASTRO_HOME names the PARENT of .astro, not .astro itself. That is what
// config.HomeConfigPath has always meant by it and what cmd/local's routesDir
// means by it, and those two are the reason it has to be read that way here: a
// user who relocates their astro home expects one relocation, not a config file
// in the new place and an env file beside it in a directory of its own.
//
// This function used to join "env" straight onto ASTRO_HOME, which put the file
// at $ASTRO_HOME/env while everything else went to $ASTRO_HOME/.astro/*. The
// doc comment above it claimed it honored the variable "the same way the rest of
// the CLI does" the whole time. Only the ASTRO_HOME-set case was ever wrong; with
// it unset both spellings land on ~/.astro/env, which is why it went unnoticed.
//
// pkg/secrets deliberately does NOT follow ASTRO_HOME at all — see its home.go
// for why a vault shared with a GUI app cannot have a movable root. That is an
// opt-out with a stated reason, not a fourth reading.
func GlobalEnvPath() (string, error) {
	home := os.Getenv("ASTRO_HOME")
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = h
	}
	return filepath.Join(home, astroDirName, "env"), nil
}

// astroDirName is the .astro directory every astro home contains. Spelled here
// rather than taken from config.ConfigDir because v2 packages do not import
// config/ (see internal/archlint).
const astroDirName = ".astro"
