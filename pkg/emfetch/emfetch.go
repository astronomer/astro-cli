// Package emfetch holds the decisions behind reading Astronomer Environment
// Manager objects: how a list endpoint is paged, how the organization-level
// refusal of secret values is recognized, and what a caller does when that
// refusal arrives.
//
// It holds no object model and no transport. The Environment Manager response
// model reaches each consumer through that consumer's own generated client,
// and a request arrives here as a closure, so the same decisions serve a
// caller whatever it speaks to the platform through and this module stays a
// leaf.
package emfetch

import (
	"bytes"
	"context"
	"errors"
	"net/http"
)

// ErrOrgSecretsRefused reports that the organization does not permit resolving
// secret values through the Environment Manager API, so a request carrying
// showSecrets was rejected. Structural objects are still readable without it.
//
// A caller's page closure returns this by classifying its response with
// RefusalFor, and WithSecretsFallback acts on it. The remediation a user reads
// is the caller's own, since where to send them differs by surface.
var ErrOrgSecretsRefused = errors.New("environment secrets fetching is not enabled for this organization")

// RefusalFor classifies one list response for a caller's page closure. It
// returns ErrOrgSecretsRefused when the response is the organization refusing a
// request that asked for secret values, and nil for anything else, leaving the
// caller to report the response on its own terms.
//
// The wantSecrets gate belongs to the classification rather than the caller: a
// refusal to resolve secrets cannot be the reason a request that never asked
// for them failed, and returning it anyway sends a user to change an
// organization setting that had no bearing on what broke.
func RefusalFor(wantSecrets bool, status int, body []byte) error {
	if !wantSecrets || !IsOrgSecretsRefusal(status, body) {
		return nil
	}
	return ErrOrgSecretsRefused
}

// IsOrgSecretsRefusal reports whether a failed list response is the
// organization refusing to resolve secret values.
//
// It reads the raw response body rather than a parsed message field, and
// accepts the refusal under any failure status. Neither is incidental: the
// platform has answered this refusal with more than one status code, and the
// body is not guaranteed to be the JSON envelope the success path parses.
// Narrowing either turns a refusal a caller can recover from into an
// unexplained failure, and the failure is silent, because the tokens that
// identify it are exactly what gets dropped.
//
// Most callers want RefusalFor, which adds the gate on having asked.
func IsOrgSecretsRefusal(status int, body []byte) bool {
	if status < http.StatusBadRequest {
		return false
	}
	lowered := bytes.ToLower(body)
	return namesShowSecretsOutsideAQuery(lowered) &&
		bytes.Contains(lowered, []byte("organization")) &&
		bytes.Contains(lowered, []byte("not allowed"))
}

// namesShowSecretsOutsideAQuery reports whether body mentions showSecrets
// somewhere other than as a query parameter.
//
// The request is a GET on /organizations/{org}/environment-objects carrying
// showSecrets, and "not allowed" is the reason phrase of both 403 and 405. So a
// gateway or proxy error page that merely quotes the request URI carries all
// three tokens without being the refusal, and in that shape showSecrets is
// always followed by "=". Requiring one mention that is not a parameter keeps
// the raw-body read while declining the echo.
func namesShowSecretsOutsideAQuery(lowered []byte) bool {
	token := []byte("showsecrets")
	for from := 0; from < len(lowered); {
		i := bytes.Index(lowered[from:], token)
		if i < 0 {
			return false
		}
		after := from + i + len(token)
		if after >= len(lowered) || lowered[after] != '=' {
			return true
		}
		from = after
	}
	return false
}

// WithSecretsFallback reads through list, asking for secret values when
// wantSecrets is set, and asks again without them if the organization refuses.
//
// The returned bool reports that the read asked for secret values and was not
// refused at the organization level. It is not a promise that every value is
// populated: the platform can still withhold an individual secret, which stays
// a per-value check for the caller. It is false whenever the fallback ran, so a
// caller that needs real values can fail on it rather than on their absence.
//
// list reports the refusal by returning ErrOrgSecretsRefused, wrapped or bare,
// which RefusalFor produces. Any other error is returned as it came. When the
// second read fails too, both causes are returned joined, so the caller can
// still say the organization refused secrets as well as what went wrong next.
func WithSecretsFallback[T any](ctx context.Context, wantSecrets bool, list func(ctx context.Context, showSecrets bool) (T, error)) (result T, secretsIncluded bool, err error) {
	var zero T
	result, err = list(ctx, wantSecrets)
	if err == nil {
		return result, wantSecrets, nil
	}
	if !wantSecrets || !errors.Is(err, ErrOrgSecretsRefused) {
		return zero, false, err
	}
	refusal := err
	result, err = list(ctx, false)
	if err != nil {
		return zero, false, errors.Join(refusal, err)
	}
	return result, false, nil
}
