package imagebuild

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// Engine is one container engine: the CLI an image action shells out to
// (Bin), the env that reaches its daemon (Env), and the runner every command
// goes through (Cmd). A build takes the same Bin and Env on its Request and
// the same Commander in New; the other image actions, logging in to a
// registry, tagging, pushing and reading a label, run on an Engine. A caller
// that resolves its engine once therefore builds, logs in and pushes on one
// engine, and an image built on one engine is never pushed from another.
//
// Every action is the engine's own CLI, `<bin> login`, `<bin> tag`,
// `<bin> push`, `<bin> image inspect`, so docker and podman are driven the same
// way and credentials live where `<bin> login` keeps them.
type Engine struct {
	Cmd Commander
	Bin string
	Env []string
}

// ErrPushForbidden reports a push the registry refused as unauthorized: a 403
// on the push, or a login before it that the registry rejected. The error a
// push returns for it wraps both this sentinel and the engine's own failure,
// so errors.Is finds the one and the other is not lost. What to advise is the
// caller's: this says nothing about any one product's registries.
var ErrPushForbidden = errors.New("failed to push image due to authentication error (403 Forbidden)")

// forbiddenError is ErrPushForbidden over the engine's own failure.
type forbiddenError struct{ cause error }

func (e *forbiddenError) Error() string {
	return ErrPushForbidden.Error() + ": " + e.cause.Error()
}
func (e *forbiddenError) Unwrap() []error { return []error{ErrPushForbidden, e.cause} }

// bearerPrefix is dropped from a login secret given as an HTTP Authorization
// value: the registry takes the token alone.
const bearerPrefix = "Bearer "

// Login logs the engine in to registry, as `<bin> login` does, and leaves the
// credential where the engine keeps it, so a later push or pull uses it.
//
// With a username, secret is written to the command's stdin
// (--password-stdin), never to its arguments, and never into an error; a
// "Bearer " prefix on it is dropped. With neither, the engine checks the
// credential it already holds for registry, and fails rather than prompt for
// one when that is missing or refused. A secret without a username is
// refused: there is no login to make with it.
func (e Engine) Login(ctx context.Context, registry, username, secret string) error {
	_, err := e.login(ctx, registry, username, secret)
	return err
}

// login is Login, also reporting whether the registry refused the
// credential: what the engine said, not the command it ran, says so.
func (e Engine) login(ctx context.Context, registry, username, secret string) (refused bool, err error) {
	args := []string{"login", registry}
	var stdin io.Reader
	switch {
	case username != "":
		args = append(args, "-u", username, "--password-stdin")
		stdin = strings.NewReader(strings.TrimPrefix(secret, bearerPrefix))
	case secret != "":
		return false, fmt.Errorf("logging in to %s: a secret was given without a username", registry)
	}
	var stderr bytes.Buffer
	if runErr := e.Cmd.Run(ctx, e.Env, rt.Stdio{In: stdin, Err: &stderr}, e.Bin, args...); runErr != nil {
		said := engineSaid(runErr, stderr.String(), registry)
		return loginRefused(said), commandError(e.Bin+" login "+registry, runErr, stderr.String(), secret)
	}
	return false, nil
}

// Tag points dst at the image src names.
func (e Engine) Tag(ctx context.Context, src, dst string) error {
	var stderr bytes.Buffer
	if err := e.Cmd.Run(ctx, e.Env, rt.Stdio{Err: &stderr}, e.Bin, "tag", src, dst); err != nil {
		return commandError(fmt.Sprintf("%s tag %s %s", e.Bin, src, dst), err, stderr.String(), "")
	}
	return nil
}

// Untag removes the name ref from the local store. The image stays while any
// other name points at it.
func (e Engine) Untag(ctx context.Context, ref string) error {
	var stderr bytes.Buffer
	if err := e.Cmd.Run(ctx, e.Env, rt.Stdio{Err: &stderr}, e.Bin, "rmi", ref); err != nil {
		return commandError(e.Bin+" rmi "+ref, err, stderr.String(), "")
	}
	return nil
}

// Label reads one label off a local image. It errors when the image is not in
// the local store or cannot be inspected, and returns "" for a label the image
// does not carry.
func (e Engine) Label(ctx context.Context, image, label string) (string, error) {
	var out, stderr bytes.Buffer
	format := fmt.Sprintf("{{ index .Config.Labels %q }}", label)
	if err := e.Cmd.Run(ctx, e.Env, rt.Stdio{Out: &out, Err: &stderr}, e.Bin, "image", "inspect", "--format", format, image); err != nil {
		return "", commandError(e.Bin+" image inspect "+image, err, stderr.String(), "")
	}
	return strings.TrimSpace(out.String()), nil
}

// PushRequest is one image pushed to a registry.
type PushRequest struct {
	// Source is the local image to push. It is tagged Image first; empty
	// pushes Image as it is.
	Source string
	// Image is the reference pushed: registry/repository:tag. Its name is
	// removed from the local store once the push is done (Untag), so a push
	// leaves no remote-named tag behind; an image built under that name alone
	// goes with it.
	Image string
	// Username and Secret, when set, log the engine in to Image's registry
	// before the push (Login), and the push uses that credential. Unset, the
	// push uses the credential the engine already holds for the registry.
	Username string
	Secret   string
	// Digest asks for the digest of the manifest pushed (sha256:…).
	Digest bool
	// Out takes the push's progress as the engine CLI writes it; nil drops
	// it. The engine's stderr is kept for the error a failed push returns.
	Out io.Writer
}

// Push pushes an image to its registry on the engine and returns the pushed
// manifest's digest when req.Digest asks for it, "" otherwise.
//
// A registry that refuses the login or the push as unauthorized fails it
// with ErrPushForbidden; any other failure carries the engine's own output.
//
// The digest is the one the registry holds. Docker records it among the
// image's repo digests once the push is done, and it is read from there.
// Podman does not record it (its repo digests are the local manifest's, which
// a push may re-encode), so a podman push writes it to a --digestfile instead.
func (e Engine) Push(ctx context.Context, req PushRequest) (string, error) {
	if req.Source != "" && req.Source != req.Image {
		if err := e.Tag(ctx, req.Source, req.Image); err != nil {
			return "", err
		}
	}
	if req.Username != "" || req.Secret != "" {
		registry, err := registryOf(req.Image)
		if err != nil {
			return "", err
		}
		if refused, err := e.login(ctx, registry, req.Username, req.Secret); err != nil {
			if refused {
				return "", &forbiddenError{cause: err}
			}
			return "", err
		}
	}

	args := []string{"push"}
	digestFile := ""
	if req.Digest && IsPodman(ctx, e.Cmd, e.Bin, e.Env) {
		f, err := os.CreateTemp("", "astro-push-digest-*")
		if err != nil {
			return "", fmt.Errorf("creating the push's digest file: %w", err)
		}
		digestFile = f.Name()
		f.Close()
		defer os.Remove(digestFile) //nolint:errcheck // best-effort cleanup of a temp file
		args = append(args, "--digestfile", digestFile)
	}
	args = append(args, req.Image)
	var stderr bytes.Buffer
	if runErr := e.Cmd.Run(ctx, e.Env, rt.Stdio{Out: req.Out, Err: &stderr}, e.Bin, args...); runErr != nil {
		err := commandError(e.Bin+" push "+req.Image, runErr, stderr.String(), req.Secret)
		if pushForbidden(engineSaid(runErr, stderr.String(), req.Image, repository(req.Image))) {
			return "", &forbiddenError{cause: err}
		}
		return "", err
	}

	digest := ""
	if req.Digest {
		var err error
		if digestFile != "" {
			digest, err = readDigestFile(digestFile, req.Image)
		} else {
			digest, err = e.repoDigest(ctx, req.Image)
		}
		if err != nil {
			return "", err
		}
	}
	if err := e.Untag(ctx, req.Image); err != nil {
		return "", err
	}
	return digest, nil
}

// repoDigest reads the digest docker recorded for image's repository when it
// pushed it.
func (e Engine) repoDigest(ctx context.Context, image string) (string, error) {
	var out, stderr bytes.Buffer
	if err := e.Cmd.Run(ctx, e.Env, rt.Stdio{Out: &out, Err: &stderr}, e.Bin, "image", "inspect", "--format", "{{json .RepoDigests}}", image); err != nil {
		return "", fmt.Errorf("failed to get digests for image %s: %w", image, commandError(e.Bin+" image inspect "+image, err, stderr.String(), ""))
	}
	var digests []string
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &digests); err != nil {
		return "", fmt.Errorf("failed to parse digests for image %s: %w", image, err)
	}
	repo := repository(image)
	for _, d := range digests {
		if name, digest, ok := strings.Cut(d, "@"); ok && name == repo {
			return digest, nil
		}
	}
	return "", fmt.Errorf("no digest was recorded for %s after pushing %s", repo, image)
}

// readDigestFile reads the digest podman wrote for a push of image.
func readDigestFile(path, image string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading the digest of %s: %w", image, err)
	}
	digest := strings.TrimSpace(string(data))
	if digest == "" {
		return "", fmt.Errorf("the push of %s wrote no digest", image)
	}
	return digest, nil
}

// registryOf is the registry host an image reference is pushed to: the part
// before the first "/". A reference without one names no registry.
func registryOf(image string) (string, error) {
	host, _, ok := strings.Cut(image, "/")
	if !ok || host == "" {
		return "", fmt.Errorf("the image %q names no registry to log in to", image)
	}
	return host, nil
}

// repository is an image reference without its tag or digest.
func repository(image string) string {
	ref, _, _ := strings.Cut(image, "@")
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref = ref[:i]
	}
	return ref
}

// commandError is a failed engine command: what ran, how it ended, and what
// it wrote to stderr, with secret (and secret without a "Bearer " prefix)
// redacted from that output.
func commandError(what string, err error, stderr, secret string) error {
	msg := strings.TrimSpace(stderr)
	for _, s := range []string{secret, strings.TrimPrefix(secret, bearerPrefix)} {
		if s != "" {
			msg = strings.ReplaceAll(msg, s, "[redacted]")
		}
	}
	if msg == "" {
		return fmt.Errorf("%s failed: %w", what, err)
	}
	return fmt.Errorf("%s failed: %w: %s", what, err, msg)
}

var (
	http403 = regexp.MustCompile(`\b403\b`)
	http401 = regexp.MustCompile(`\b401\b`)
)

// engineSaid is what a failed engine command said about its failure: its
// stderr and the error it ended with, never the command line. The references
// it ran on are cut out, so a name that happens to hold "403" or "forbidden"
// (a tag such as deploy-403) is not read as the registry's answer.
func engineSaid(runErr error, stderr string, refs ...string) string {
	said := stderr + "\n" + runErr.Error()
	for _, ref := range refs {
		if ref != "" {
			said = strings.ReplaceAll(said, ref, "")
		}
	}
	return strings.ToLower(said)
}

// pushForbidden reports a push the registry refused as unauthorized: a 403, a
// "forbidden" or an "authentication required" in what the engine said.
func pushForbidden(said string) bool {
	return http403.MatchString(said) || strings.Contains(said, "forbidden") || strings.Contains(said, "authentication required")
}

// loginRefused reports a login the registry refused the credential of: what
// pushForbidden reports, or a 401, "unauthorized" or "incorrect username or
// password".
func loginRefused(said string) bool {
	return pushForbidden(said) || http401.MatchString(said) || strings.Contains(said, "unauthorized") ||
		strings.Contains(said, "incorrect username or password")
}
