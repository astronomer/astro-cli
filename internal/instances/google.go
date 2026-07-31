package instances

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/oauth2/google"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
)

// googleScope is what an access token for Composer's Airflow has to cover.
// Composer accepts a plain Google OAuth2 access token and cloud-platform is
// the scope gcloud's application-default login already grants, so nothing on a
// working machine needs re-consenting.
const googleScope = "https://www.googleapis.com/auth/cloud-platform"

// ErrNoGoogleCredentials reports a machine with no Application Default
// Credentials: no gcloud ADC file, no GOOGLE_APPLICATION_CREDENTIALS, no
// metadata server. One fix covers the laptop case, which is the case a person
// is in when they read this. It is exported so the Composer URL lookup, which
// needs the same credentials, reports the same outage.
var ErrNoGoogleCredentials = errors.New("no Google credentials — run `gcloud auth application-default login`")

// maxComposerAccountLength is the longest service-account email an Airflow that
// registers Google callers on its own can store. The token carries the full
// email as the Airflow username and Airflow's username column stops at 64
// characters, so a longer service account has to be registered ahead of time
// under its numeric account id or every call comes back 403.
//
// It is Airflow's limit rather than Composer's, which is why the advice below
// does not name Composer: a self-hosted Airflow behind Google IAP hits the same
// wall, and Composer is only where it turns up most.
const maxComposerAccountLength = 64

// GoogleAccountAdvice is the fix for a service account whose email is too long
// for Composer's Airflow to register, or the empty string when the account is
// not one. It reads as a sentence continuing a 403: the caller supplies the
// refusal, this supplies the cause.
//
// It is exported because both 403s want it — the Composer API's, which
// internal/instancelocate reports, and the Airflow's own, which the google
// refresh hook here reports — and two wordings of one fix would be one too
// many. An impersonated or workload-identity credential whose ADC document
// names no account at all is invisible to this, and reads as nothing to warn
// about.
func GoogleAccountAdvice(account string) string {
	if len(account) <= maxComposerAccountLength {
		return ""
	}
	return fmt.Sprintf("the service account %s is longer than %d characters, which is more than Airflow can store as a username — "+
		"pre-register it as an Airflow user under its numeric account id, or use a shorter service account",
		account, maxComposerAccountLength)
}

// googleCredentials proves the caller with Application Default Credentials —
// the chain gcloud, a service-account key file, and workload identity all feed.
// Nothing is stored: the token source refreshes underneath the credential
// source, and the token itself lives in memory for the run.
//
// The second return is the refresh hook, and it is where Composer's one
// genuinely surprising refusal gets named. A 403 from a Composer Airflow says
// nothing about why, and the likeliest why — a service account whose email is
// too long for Airflow's username column — is invisible in the answer and
// visible in the credentials. The hook runs on exactly the 401 and 403 that
// mean "these credentials did not work", so it is the one place an Airflow
// status and the identity behind it are both in hand.
func googleCredentials(d Deps) (source airflowapi.CredentialSource, refresh func(context.Context) error) {
	source = func(ctx context.Context) (string, string, error) {
		token, err := d.googleToken(ctx)
		if err != nil {
			return "", "", err
		}
		return airflowapi.BearerToken(token)(ctx)
	}
	refresh = func(ctx context.Context) error {
		if advice := GoogleAccountAdvice(d.googleAccount(ctx)); advice != "" {
			return errors.New(advice)
		}
		// Nothing to add. The ADC token source renews on its own, so the retry
		// carries whatever it has and the status stands as Airflow's answer.
		return nil
	}
	return source, refresh
}

// googleToken hands back an ADC access token, through the seam when one is
// wired and from the SDK's own chain otherwise.
func (d Deps) googleToken(ctx context.Context) (string, error) {
	if d.GoogleToken != nil {
		return d.GoogleToken(ctx)
	}
	return defaultADC.token(ctx)
}

// googleAccount names the principal those credentials speak for, through the
// same seam.
func (d Deps) googleAccount(ctx context.Context) string {
	if d.GoogleAccount != nil {
		return d.GoogleAccount(ctx)
	}
	return defaultADC.account(ctx)
}

// GoogleAccessToken is an Application Default Credentials access token from
// the machine's own chain. It is exported because the Composer URL lookup
// needs the same token the Airflow calls after it carry, and one
// implementation means one place a missing chain is named.
func GoogleAccessToken(ctx context.Context) (string, error) {
	return defaultADC.token(ctx)
}

// GoogleAccount is the principal the machine's Application Default Credentials
// speak for — a service account's email when the credentials name one, empty
// for a user login, which has no such limit to run into. It is what makes the
// over-long-service-account failure identifiable rather than guessed at.
func GoogleAccount(ctx context.Context) string {
	return defaultADC.account(ctx)
}

// adc finds Application Default Credentials once per process. The lookup reads
// files and may call the metadata server, and every instance and lookup in a
// run wants the same answer, so it is worth holding.
//
// Only a success is held. A failure that came from a canceled context is not a
// fact about the machine, and caching one would make the first command to be
// interrupted poison every lookup after it — the same reason
// airflowapi.Client.detect caches only what it proved.
//
// It is process-global because Application Default Credentials are a property
// of the process, not of any one instance: two links resolving the same laptop
// twice would be waste, and there is nothing per-instance to key it on.
type adc struct {
	mu     sync.Mutex
	creds  *google.Credentials
	looked bool
}

var defaultADC = &adc{}

func (a *adc) find(ctx context.Context) (*google.Credentials, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.looked {
		return a.creds, nil
	}
	// Detached from the caller's context deliberately. FindDefaultCredentials
	// reads files and may ask the metadata server, and the answer it produces
	// is held for the process; letting one command's cancellation decide what
	// every later lookup sees would be the wrong lifetime entirely. The
	// metadata probe carries its own short timeout.
	creds, err := google.FindDefaultCredentials(context.WithoutCancel(ctx), googleScope)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoGoogleCredentials, err)
	}
	a.creds, a.looked = creds, true
	return creds, nil
}

func (a *adc) token(ctx context.Context) (string, error) {
	creds, err := a.find(ctx)
	if err != nil {
		return "", err
	}
	token, err := creds.TokenSource.Token()
	if err != nil {
		// Credentials were found and would not turn into a token: a revoked
		// key, a refresh that could not reach Google. Logging in again is not
		// the fix here, so this does not say it is.
		return "", fmt.Errorf("could not get a Google access token from the credentials on this machine: %w", err)
	}
	return token.AccessToken, nil
}

// account reads the principal out of the credential JSON: a service-account key
// names it outright, and an impersonated or workload-identity credential names
// it inside the URL it impersonates through. A user login has neither, so an
// empty answer means "nothing to warn about".
func (a *adc) account(ctx context.Context) string {
	creds, err := a.find(ctx)
	if err != nil || creds == nil || len(creds.JSON) == 0 {
		return ""
	}
	return accountFromADC(creds.JSON)
}

// accountFromADC pulls the service-account email out of an ADC document.
func accountFromADC(payload []byte) string {
	var doc struct {
		ClientEmail string `json:"client_email"`
		// ImpersonationURL is what an impersonated or workload-identity
		// credential carries instead: the IAM endpoint it mints through, whose
		// path holds the account being impersonated.
		ImpersonationURL string `json:"service_account_impersonation_url"`
	}
	if err := json.Unmarshal(payload, &doc); err != nil {
		return ""
	}
	if doc.ClientEmail != "" {
		return doc.ClientEmail
	}
	return accountFromImpersonationURL(doc.ImpersonationURL)
}

// impersonationMarker precedes the impersonated account in the IAM URL, which
// ends ".../serviceAccounts/<email>:generateAccessToken".
const impersonationMarker = "/serviceAccounts/"

func accountFromImpersonationURL(raw string) string {
	_, after, found := strings.Cut(raw, impersonationMarker)
	if !found {
		return ""
	}
	account, _, _ := strings.Cut(after, ":")
	return account
}
