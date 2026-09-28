package manifest

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// [tool.astro] build-secrets lists docker build --secret specs for the
// declared Dockerfile's build, the same specs --build-secret takes. A spec
// names where a secret comes from, an environment variable or a file, and
// never holds the secret, so the list is safe to commit. A --build-secret flag
// replaces the list, and so does BUILD_SECRET_INPUT when no flag is given.

const buildSecretsKey = astroRoot + ".build-secrets"

// typeEnv is both the key naming a variable and the type that makes src=
// name one.
const typeEnv = "env"

// ErrBuildSecretEnvNotAName is an env= that cannot be a variable name. The
// likeliest cause is a shell that expanded $VAR into the spec, which puts the
// secret itself where its variable's name belongs.
var ErrBuildSecretEnvNotAName = errors.New("has an env= that is not an environment variable name. Write the variable's name, not $VAR, which the shell replaces with the secret")

var envVarName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// BuildSecret is one docker build --secret spec, read the way docker reads it.
type BuildSecret struct {
	ID string
	// Env is the environment variable the build reads the secret from.
	Env string
	// Src is the file the build reads the secret from.
	Src string
}

// ParseBuildSecret reads a --build-secret spec: comma-separated id=, src= (or
// source=), env= and type= pairs, the keys docker reads. type=env makes src=
// name a variable. It refuses a field that is not a key=value pair and a key
// docker does not know. Unlike docker it trims spaces and does not read
// quoted fields, and it refuses an env= that is not a variable name. The errors repeat nothing from the spec, since a mistaken
// spec can be the secret itself.
func ParseBuildSecret(spec string) (BuildSecret, error) {
	var s BuildSecret
	var typ string
	for _, field := range strings.Split(strings.TrimSpace(spec), ",") {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			return BuildSecret{}, errors.New("has a field that is not a key=value pair")
		}
		value = strings.TrimSpace(value)
		switch key = strings.ToLower(strings.TrimSpace(key)); key {
		case "id":
			s.ID = value
		case "src", "source":
			s.Src = value
		case typeEnv:
			s.Env = value
		case "type":
			if value != "file" && value != typeEnv {
				return BuildSecret{}, errors.New("has a type other than file or env")
			}
			typ = value
		default:
			return BuildSecret{}, errors.New("has a key other than id, src, env and type")
		}
	}
	if typ == typeEnv && s.Env == "" {
		s.Env, s.Src = s.Src, ""
	}
	if s.Env != "" && !envVarName.MatchString(s.Env) {
		return BuildSecret{}, ErrBuildSecretEnvNotAName
	}
	return s, nil
}

// String is the spec docker reads.
func (s BuildSecret) String() string {
	if s.Env != "" {
		return "id=" + s.ID + ",env=" + s.Env
	}
	return "id=" + s.ID + ",src=" + s.Src
}

// BuildSecretSpecs is build-secrets as docker reads them. A src= that starts
// with ~/ is joined to the home directory, since docker does not expand it.
func (a *Astro) BuildSecretSpecs() []string {
	if len(a.BuildSecrets) == 0 {
		return nil
	}
	home, homeErr := os.UserHomeDir()
	out := make([]string, 0, len(a.BuildSecrets))
	for _, spec := range a.BuildSecrets {
		s, err := ParseBuildSecret(spec)
		if err != nil {
			out = append(out, spec)
			continue
		}
		if rest, ok := strings.CutPrefix(s.Src, "~/"); ok && homeErr == nil {
			s.Src = filepath.Join(home, rest)
		}
		out = append(out, s.String())
	}
	return out
}

// buildSecrets decodes [tool.astro] build-secrets and checks each entry. A
// src= has to be absolute or start with ~/: a relative path would be read
// from wherever the command runs, and a secret file inside the project would
// be in the build context too.
func (p *parser) buildSecrets(v any) []string {
	return p.stringArray(buildSecretsKey, v, func(key, spec string) {
		s, err := ParseBuildSecret(spec)
		switch {
		case err != nil:
			p.add(CodeBuildSecretInvalid, key, "is not a build secret spec: it "+err.Error()+". "+specShape)
		case s.ID == "":
			p.add(CodeBuildSecretInvalid, key, "names no id. "+specShape)
		case (s.Env == "") == (s.Src == ""):
			p.add(CodeBuildSecretInvalid, key, "has to name exactly one source, env= or src=. "+specShape)
		case s.Src != "" && !filepath.IsAbs(s.Src) && !strings.HasPrefix(s.Src, "/") && !strings.HasPrefix(s.Src, "~/"):
			p.add(CodeBuildSecretInvalid, key, "has a src= that is neither absolute nor under ~/, so it would be read relative to wherever the command runs")
		}
	})
}

const specShape = "Write id=<name>,env=<VAR> or id=<name>,src=<file>; the spec names where the secret comes from, never the secret"
