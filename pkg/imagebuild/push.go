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
	"sync"

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
// way.
type Engine struct {
	Cmd Commander
	Bin string
	Env []string
}

// ErrPushForbidden reports a push the registry refused as unauthorized. A
// push returns it as a *PushForbiddenError, which says whose credential was
// refused and carries the engine's own failure. What to advise is the
// caller's: this says nothing about any one product's registries.
var ErrPushForbidden = errors.New("failed to push image due to authentication error (403 Forbidden)")

// PushForbiddenError is a push the registry refused as unauthorized. It
// matches ErrPushForbidden with errors.Is.
type PushForbiddenError struct {
	// WithCredential is true when the push carried its own credential
	// (PushRequest.Username and Secret) and the registry refused that, false
	// when the credential refused was the one the engine holds for the
	// registry in the user's own config.
	WithCredential bool
	// Err is the engine's failure: the command and what it said, with the
	// push's secret redacted.
	Err error
}

func (e *PushForbiddenError) Error() string {
	return ErrPushForbidden.Error() + ": " + e.Err.Error()
}

func (e *PushForbiddenError) Unwrap() []error { return []error{ErrPushForbidden, e.Err} }

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
		return errSecretWithoutUser(registry)
	}
	var stderr bytes.Buffer
	if err := e.Cmd.Run(ctx, e.Env, rt.Stdio{In: stdin, Err: &stderr}, e.Bin, args...); err != nil {
		return commandError(e.Bin+" login "+registry, err, stderr.String(), secret)
	}
	return nil
}

func errSecretWithoutUser(registry string) error {
	return fmt.Errorf("logging in to %s: a secret was given without a username", registry)
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
	// Username and Secret, when set, are the credential the push uses for
	// Image's registry. No login is run: they are written to a credential
	// file made for this one push and deleted after it, success or not (see
	// pushAuth), never the user's own credential store, so nothing is left
	// behind and concurrent pushes never share one. A "Bearer " prefix on
	// Secret is dropped. Unset, the push uses the engine's own config and the
	// credential it already holds for the registry, as any `<bin> push` does.
	Username string
	Secret   string
	// Digest asks for the digest of the manifest pushed (sha256:…).
	Digest bool
	// Out takes the push's progress as the engine CLI writes it, from its
	// stdout and its stderr both, whole lines at a time and with Secret
	// redacted; nil drops it. Docker writes its progress to stdout and podman
	// to stderr ("Copying blob …"), so either engine's shows. The stderr is
	// kept as well, for the error a failed push returns. A write to Out that
	// fails does not fail the push.
	Out io.Writer
}

// Push pushes an image to its registry on the engine and returns the pushed
// manifest's digest when req.Digest asks for it, "" otherwise.
//
// A registry that refuses the push as unauthorized fails it with a
// *PushForbiddenError (ErrPushForbidden); any other failure carries the
// engine's own output.
//
// The digest is the one the registry holds. Docker records it among the
// image's repo digests once the push is done, and it is read from there.
// Podman does not record it (its repo digests are the local manifest's, which
// a push may re-encode), so a podman push writes it to a --digestfile instead.
func (e Engine) Push(ctx context.Context, req PushRequest) (string, error) {
	// Checked before anything runs, so a refused request leaves no tag.
	registry, err := credentialRegistry(&req)
	if err != nil {
		return "", err
	}
	credential := registry != ""
	untag := false
	if req.Source != "" && req.Source != req.Image {
		if err := e.Tag(ctx, req.Source, req.Image); err != nil {
			return "", err
		}
		// The remote name is ours from here: dropped however the push ends,
		// even canceled, as well as on success below.
		untag = true
		defer func() {
			if untag {
				e.Untag(context.WithoutCancel(ctx), req.Image) //nolint:errcheck // best-effort on a failed push
			}
		}()
	}
	podman := (credential || req.Digest) && IsPodman(ctx, e.Cmd, e.Bin, e.Env)
	auth := &pushAuth{}
	if credential {
		if auth, err = newPushAuth(registry, req.Username, req.Secret, podman, e.Env); err != nil {
			return "", err
		}
		defer auth.remove()
	}

	var args []string
	digestFile := ""
	if req.Digest && podman {
		if digestFile, err = newDigestFile(); err != nil {
			return "", err
		}
		defer os.Remove(digestFile) //nolint:errcheck // best-effort cleanup of a temp file
		args = append(args, "--digestfile", digestFile)
	}
	args = auth.cmdArgs("push", append(args, req.Image)...)
	var stderr bytes.Buffer
	if runErr := e.runPush(ctx, auth.cmdEnv(e.Env), &req, &stderr, args); runErr != nil {
		return "", e.pushError(&req, credential, runErr, stderr.String())
	}

	digest := ""
	if req.Digest {
		if digest, err = e.pushedDigest(ctx, req.Image, digestFile); err != nil {
			return "", err
		}
	}
	untag = false
	if err := e.Untag(ctx, req.Image); err != nil {
		return "", err
	}
	return digest, nil
}

// runPush runs the push command: its stdout and stderr both to req.Out, as
// whole lines with req.Secret redacted, and its stderr kept in stderr as well.
func (e Engine) runPush(ctx context.Context, env []string, req *PushRequest, stderr *bytes.Buffer, args []string) error {
	if req.Out == nil {
		return e.Cmd.Run(ctx, env, rt.Stdio{Err: stderr}, e.Bin, args...)
	}
	progress := &progressLines{w: req.Out, secret: req.Secret}
	stdout, errLines := progress.stream(), progress.stream()
	err := e.Cmd.Run(ctx, env, rt.Stdio{Out: stdout, Err: io.MultiWriter(stderr, errLines)}, e.Bin, args...)
	stdout.flush()
	errLines.flush()
	return err
}

// credentialRegistry is the registry a push that carries its own credential
// is authenticated to, "" for a push that carries none. A credential for a
// reference that names no registry, or a secret without a username, is
// refused.
func credentialRegistry(req *PushRequest) (string, error) {
	if req.Username == "" && req.Secret == "" {
		return "", nil
	}
	registry, err := registryOf(req.Image)
	if err != nil {
		return "", err
	}
	if req.Username == "" {
		return "", errSecretWithoutUser(registry)
	}
	return registry, nil
}

// pushError is a failed push: a *PushForbiddenError when what the engine said
// is a refusal of the credential (its own, when it carried one), its plain
// failure otherwise. The secret is redacted from both.
func (e Engine) pushError(req *PushRequest, credential bool, runErr error, stderr string) error {
	err := commandError(e.Bin+" push "+req.Image, runErr, stderr, req.Secret)
	said := engineSaid(runErr, stderr, req.Image, repository(req.Image))
	if pushForbidden(said) || (credential && credentialRefused(said)) {
		return &PushForbiddenError{WithCredential: credential, Err: err}
	}
	return err
}

// pushedDigest is the digest of the manifest just pushed as image: podman's
// digestFile when it wrote one, docker's recorded repo digest otherwise.
func (e Engine) pushedDigest(ctx context.Context, image, digestFile string) (string, error) {
	if digestFile != "" {
		return readDigestFile(digestFile, image)
	}
	return e.repoDigest(ctx, image)
}

// newDigestFile is an empty temporary file for podman to write a push's
// digest to.
func newDigestFile() (string, error) {
	f, err := os.CreateTemp("", "astro-push-digest-*")
	if err != nil {
		return "", fmt.Errorf("creating the push's digest file: %w", err)
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		os.Remove(name) //nolint:errcheck // best-effort cleanup of a temp file
		return "", err
	}
	return name, nil
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

// registryOf is the registry an image reference is pushed to, read as docker
// reads a reference: the first component is a registry only when
// it holds a "." or a ":" or is "localhost"; anything else (alpine:3,
// org/repo:t) is on Docker Hub, docker.io.
func registryOf(image string) (string, error) {
	if image == "" {
		return "", errors.New("no image was given to push")
	}
	host, _, ok := strings.Cut(image, "/")
	if !ok || (!strings.ContainsAny(host, ".:") && host != "localhost") {
		return dockerHub, nil
	}
	return host, nil
}

// dockerHub is Docker Hub's registry as registryOf names it.
const dockerHub = "docker.io"

// authKeys are the keys a credential for registry is stored under so that
// both engines find it. Docker looks Docker Hub up under its legacy index
// URL, containers under docker.io; any other registry is its host for both.
func authKeys(registry string) []string {
	if registry == dockerHub || registry == "index.docker.io" {
		return []string{"https://index.docker.io/v1/", dockerHub}
	}
	return []string{registry}
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
	msg := redact(strings.TrimSpace(stderr), secret)
	if msg == "" {
		return fmt.Errorf("%s failed: %w", what, err)
	}
	return fmt.Errorf("%s failed: %w: %s", what, err, msg)
}

// redact is s with secret, and secret without a "Bearer " prefix, replaced.
func redact(s, secret string) string {
	for _, v := range []string{secret, strings.TrimPrefix(secret, bearerPrefix)} {
		if v != "" {
			s = strings.ReplaceAll(s, v, "[redacted]")
		}
	}
	return s
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

// credentialRefused reports a push whose own credential the registry
// refused: what pushForbidden reports, or a 401, "unauthorized", "incorrect
// username or password" or a registry's "requested access to the resource is
// denied". Not a bare "denied", which a socket or a file the engine cannot
// open says too. A push on the user's stored credential is held to
// pushForbidden alone, as it always was.
func credentialRefused(said string) bool {
	return pushForbidden(said) || http401.MatchString(said) || strings.Contains(said, "unauthorized") ||
		strings.Contains(said, "incorrect username or password") ||
		strings.Contains(said, "requested access to the resource is denied")
}

// progressLines is the one writer a push's stdout and stderr both stream to.
// The engine writes the two from separate goroutines, so each stream gathers
// whole lines and hands them over under a lock: a line from one never lands
// in the middle of a line from the other. Each line has secret redacted
// before it is written.
type progressLines struct {
	mu     sync.Mutex
	w      io.Writer
	secret string
}

// maxProgressLine bounds how much of a line a stream holds before writing it
// anyway, so output with no newline in it is not held without limit.
const maxProgressLine = 64 << 10

// stream is a writer for one of the engine's outputs.
func (p *progressLines) stream() *progressStream { return &progressStream{dst: p} }

func (p *progressLines) write(line []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Progress is best-effort: a failed write must neither fail the push
	// nor stop the stderr it shares a writer with from being kept.
	io.WriteString(p.w, redact(string(line), p.secret)) //nolint:errcheck // best-effort progress
}

// progressStream is one output of the engine, written to its progressLines a
// whole line at a time.
type progressStream struct {
	dst *progressLines
	buf []byte
}

func (s *progressStream) Write(b []byte) (int, error) {
	s.buf = append(s.buf, b...)
	for {
		i := bytes.IndexByte(s.buf, '\n')
		if i < 0 {
			break
		}
		s.dst.write(s.buf[:i+1])
		s.buf = s.buf[i+1:]
	}
	if len(s.buf) >= maxProgressLine {
		s.flush()
	}
	return len(b), nil
}

// flush writes what the stream holds of a line not yet ended.
func (s *progressStream) flush() {
	if len(s.buf) > 0 {
		s.dst.write(s.buf)
		s.buf = nil
	}
}
