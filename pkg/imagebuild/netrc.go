package imagebuild

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// The netrc build secret, when the project names none it can be read from.
// uv lock reaches a private git or URL dependency on the host through git's
// credential helper (a keychain, gh, a credential manager), which an image
// build cannot ask. So when the lock installs from a host and no netrc source
// is given, the build asks that helper for the host's credentials, as uv did,
// and passes them to docker as the netrc secret through an environment
// variable of the build command: never a file in the project, never a layer,
// and never a line of output.

// netrcEnv is the variable the build command reads the netrc secret from when
// the build makes it.
const netrcEnv = "ASTRO_BUILD_NETRC"

// credentialTimeout bounds one credential helper call. A helper that wants to
// ask a person has nobody to ask here.
const credentialTimeout = 10 * time.Second

// CredentialSource returns the login and password for host, and false when it
// has none. GitCredentials is the one the CLI uses.
type CredentialSource func(ctx context.Context, host string) (login, password string, ok bool)

// WithCredentials sets where the build reads a host's credentials from for
// the netrc secret it makes (withNetrc). A Builder without one makes none.
func (b *Builder) WithCredentials(c CredentialSource) *Builder {
	b.creds = c
	return b
}

// GitCredentials asks git's credential helpers for https://host, the way
// `git clone` and uv would, without ever prompting: no terminal, no askpass,
// no credential manager window.
func GitCredentials(ctx context.Context, host string) (login, password string, ok bool) {
	ctx, cancel := context.WithTimeout(ctx, credentialTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-c", "credential.interactive=never", "credential", "fill")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "SSH_ASKPASS=", "GCM_INTERACTIVE=never")
	cmd.Stdin = strings.NewReader("protocol=https\nhost=" + host + "\n\n")
	var out bytes.Buffer
	cmd.Stdout = &out
	if cmd.Run() != nil {
		return "", "", false
	}
	return parseCredential(out.String())
}

// parseCredential reads git credential's key=value output.
func parseCredential(out string) (login, password string, ok bool) {
	for _, line := range strings.Split(out, "\n") {
		key, value, _ := strings.Cut(strings.TrimRight(line, "\r"), "=")
		switch key {
		case "username":
			login = value
		case "password":
			password = value
		}
	}
	return login, password, login != "" && password != "" && !strings.ContainsAny(login+password, " \t\n")
}

// withNetrc gives the build the netrc secret when it needs one and has no
// source for it: the project names netrc with no source (build-secrets
// 'id=netrc'), or names none while the lock installs from a host. ~/.netrc is
// that source when it exists; otherwise the build's credential source, for
// each host the lock or a declared Dockerfile installs from. A netrc the
// project named that leaves a host without credentials is refused before the
// build; one the build only offered is left out, since a public host needs
// none.
func (b *Builder) withNetrc(ctx context.Context, req Request) (Request, error) {
	at, declared := -1, false
	for i, spec := range req.Secrets {
		if s, err := manifest.ParseBuildSecret(spec); err == nil && s.ID == manifest.RuntimeSecretID {
			at, declared = i, s.Env == "" && s.Src == ""
			break
		}
	}
	if at >= 0 && !declared {
		return req, nil
	}
	hosts := req.netrcHosts()
	if at < 0 && len(hosts) == 0 {
		return req, nil
	}
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, ".netrc")
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			return withSecret(req, at, "id="+manifest.RuntimeSecretID+",src="+p), nil
		}
	}
	var lines, missing []string
	for _, h := range hosts {
		var login, password string
		ok := false
		if b.creds != nil {
			login, password, ok = b.creds(ctx, h)
		}
		if !ok {
			missing = append(missing, h)
			continue
		}
		lines = append(lines, "machine "+h+" login "+login+" password "+password)
	}
	switch {
	case declared && len(missing) > 0:
		return req, fmt.Errorf("the netrc build secret has no source for %s: there is no ~/.netrc, and git has no credentials for %s. "+
			"Pass --build-secret id=netrc,env=NETRC (or src=<file>) with a netrc that covers %s, or sign git in to %s",
			strings.Join(missing, ", "), pronounFor(missing), pronounFor(missing), pronounFor(missing))
	case len(lines) == 0:
		if at >= 0 {
			req.Secrets = slices.Delete(slices.Clone(req.Secrets), at, at+1)
		}
		return req, nil
	}
	req.Env = append(slices.Clone(req.Env), netrcEnv+"="+strings.Join(lines, "\n")+"\n")
	return withSecret(req, at, "id="+manifest.RuntimeSecretID+",env="+netrcEnv), nil
}

// pronounFor names hosts as the error's later clauses refer to them.
func pronounFor(hosts []string) string {
	if len(hosts) == 1 {
		return hosts[0]
	}
	return "them"
}

// withSecret puts spec in req's secrets at at, or appends it when at is -1.
func withSecret(req Request, at int, spec string) Request {
	req.Secrets = slices.Clone(req.Secrets)
	if at >= 0 {
		req.Secrets[at] = spec
	} else {
		req.Secrets = append(req.Secrets, spec)
	}
	return req
}

// dockerfileURLRe finds the hosts a declared Dockerfile installs from:
// https URLs, git+https ones included.
var dockerfileURLRe = regexp.MustCompile(`https://([A-Za-z0-9.-]+)[/:]`)

// netrcHosts are the hosts the build installs from over https, sorted: each
// direct reference and routed file of the lists (RoutedLinks), and each https URL in a
// declared Dockerfile.
func (r *Request) netrcHosts() []string {
	seen := map[string]bool{}
	add := func(raw string) {
		u, err := url.Parse(strings.TrimPrefix(raw, "git+"))
		if err == nil && u.Scheme == "https" && u.Hostname() != "" {
			seen[u.Hostname()] = true
		}
	}
	l := r.Lists()
	for _, line := range l.Requirements {
		if _, ref, ok := strings.Cut(line, " @ "); ok {
			ref, _, _ = strings.Cut(ref, ";")
			if f := strings.Fields(ref); len(f) > 0 {
				add(f[0])
			}
		}
	}
	for _, link := range l.RoutedLinks {
		add(link)
	}
	if r.Dockerfile != "" {
		if data, err := os.ReadFile(r.Dockerfile); err == nil {
			for _, m := range dockerfileURLRe.FindAllStringSubmatch(string(data), -1) {
				seen[m[1]] = true
			}
		}
	}
	hosts := make([]string, 0, len(seen))
	for h := range seen {
		hosts = append(hosts, h)
	}
	slices.Sort(hosts)
	return hosts
}
