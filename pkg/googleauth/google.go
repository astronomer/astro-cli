// Package googleauth performs the google auth method, which reaches a Composer
// Airflow with an Application Default Credentials token.
//
// Separate from awsauth, and that is the point rather than tidiness. Bundling
// both doors in one package made anything that wanted only this one link the
// AWS SDK as well: internal/instancelocate needs exactly the three helpers
// below to look up a Composer environment's URL, and it was paying 88 AWS
// packages for them.
//
// Measured against the same base, a program importing the core and one door:
// the core alone links 5.1MB, adding googleauth makes it 7.0MB, and adding
// awsauth makes it 12.0MB. So Google costs about 1.9MB and AWS about 6.9MB, and
// they are separate packages because they are separate decisions.
//
// Measure by INVOKING a door, not by constructing its provider. A probe that
// only builds the provider value reports 7.3MB for both doors, because the
// linker drops the credential chain behind a closure nothing calls — which
// under-reports AWS by two thirds.
//
// A build that never imports this package still reads manifests naming the
// method, and refuses them by name — see instances.Providers.
package googleauth

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

// ErrNoCredentials reports a machine with no Application Default
// Credentials: no gcloud ADC file, no GOOGLE_APPLICATION_CREDENTIALS, no
// metadata server. One fix covers the laptop case, which is the case a person
// is in when they read this. It is exported so the Composer URL lookup, which
// needs the same credentials, reports the same outage.
var ErrNoCredentials = errors.New("no Google credentials — run gcloud auth application-default login")

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

// AccountAdvice is the fix for a service account whose email is too long
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
func AccountAdvice(account string) string {
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
func googleCredentials(o Options) (source airflowapi.CredentialSource, refresh func(context.Context) error) {
	source = func(ctx context.Context) (string, string, error) {
		token, err := o.ResolveToken(ctx)
		if err != nil {
			return "", "", err
		}
		return airflowapi.BearerToken(token)(ctx)
	}
	refresh = func(ctx context.Context) error {
		if advice := AccountAdvice(o.ResolveAccount(ctx)); advice != "" {
			return errors.New(advice)
		}
		// Nothing to add. The ADC token source renews on its own, so the retry
		// carries whatever it has and the status stands as Airflow's answer.
		return nil
	}
	return source, refresh
}

// ResolveToken hands back an ADC access token, through the seam when one is
// wired and from the SDK's own chain otherwise.
//
// Exported because a Composer address lookup needs the same answer this door
// gives, from the same Options value: pkg/instancelocate takes one of these
// rather than restating its fields, so the lookup and the Airflow calls after
// it cannot end up on two different chains.
func (o Options) ResolveToken(ctx context.Context) (string, error) {
	if o.Token != nil {
		return o.Token(ctx)
	}
	return defaultADC.token(ctx)
}

// ResolveAccount names the principal those credentials speak for, through the
// same seam.
func (o Options) ResolveAccount(ctx context.Context) string {
	if o.Account != nil {
		return o.Account(ctx)
	}
	return defaultADC.account(ctx)
}

// AccessToken is an Application Default Credentials access token from
// the machine's own chain. It is exported because the Composer URL lookup
// needs the same token the Airflow calls after it carry, and one
// implementation means one place a missing chain is named.
func AccessToken(ctx context.Context) (string, error) {
	return defaultADC.token(ctx)
}

// Account is the principal the machine's Application Default Credentials
// speak for — a service account's email when the credentials name one, empty
// for a user login, which has no such limit to run into. It is what makes the
// over-long-service-account failure identifiable rather than guessed at.
func Account(ctx context.Context) string {
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
		return nil, fmt.Errorf("%w: %w", ErrNoCredentials, err)
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
