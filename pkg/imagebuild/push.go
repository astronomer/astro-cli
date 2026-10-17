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

// ErrPushForbidden reports a push the registry refused as unauthorized (403
// Forbidden, or a login the registry rejected). The error a push returns for
// it says only this, and wraps the engine's own failure as well, so
// errors.Is finds this sentinel and the engine's output is not lost.
var ErrPushForbidden = errors.New(`failed to push image due to authentication error (403 Forbidden).

This commonly occurs due to:
1. Invalid cached Docker credentials
2. Incompatible containerd snapshotter configuration
3. A very long upload: the push credential can expire before a large layer finishes

To resolve:
• Run 'docker logout' for each Astro registry to clear cached credentials
• Ensure containerd snapshotter is disabled (Docker Desktop users)
• If the push ran for many minutes, retry, or make the image smaller
• Try running 'astro deploy' again

For detailed troubleshooting steps, visit:
https://support.astronomer.io/hc/en-us/articles/41427905156243-403-errors-on-image-push`)

// forbiddenError is ErrPushForbidden's text over the engine's own failure.
type forbiddenError struct{ cause error }

func (e *forbiddenError) Error() string   { return ErrPushForbidden.Error() }
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
	args := []string{"login", registry}
	var stdin io.Reader
	switch {
	case username != "":
		args = append(args, "-u", username, "--password-stdin")
		stdin = strings.NewReader(strings.TrimPrefix(secret, bearerPrefix))
	case secret != "":
		return fmt.Errorf("logging in to %s: a secret was given without a username", registry)
	}
	var stderr bytes.Buffer
	if err := e.Cmd.Run(ctx, e.Env, rt.Stdio{In: stdin, Err: &stderr}, e.Bin, args...); err != nil {
		return commandError(e.Bin+" login "+registry, err, stderr.String(), secret)
	}
	return nil
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
		if err := e.Login(ctx, registry, req.Username, req.Secret); err != nil {
			if isForbidden(err) {
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
	if err := e.Cmd.Run(ctx, e.Env, rt.Stdio{Out: req.Out, Err: &stderr}, e.Bin, args...); err != nil {
		err = commandError(e.Bin+" push "+req.Image, err, stderr.String(), req.Secret)
		if isForbidden(err) {
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

var http403 = regexp.MustCompile(`\b403\b`)

// isForbidden reports a failure a registry gave as unauthorized: a 403, a
// "forbidden" or an "authentication required" anywhere in the error's chain.
func isForbidden(err error) bool {
	for ; err != nil; err = errors.Unwrap(err) {
		s := strings.ToLower(err.Error())
		if http403.MatchString(s) || strings.Contains(s, "forbidden") || strings.Contains(s, "authentication required") {
			return true
		}
	}
	return false
}
