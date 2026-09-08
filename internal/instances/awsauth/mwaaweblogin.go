package awsauth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/mwaa"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
)

// The second way into MWAA: the web-login-token exchange, for environments
// whose IAM policy predates airflow:InvokeRestAPI. CreateWebLoginToken hands
// back a sixty-second token and the web server's hostname, the token is traded
// at /aws_mwaa/login for a session cookie, and everything after that is
// ordinary HTTP to an ordinary Airflow — version prefixes and all, unlike the
// AWS API.
//
// Nothing is written down. The token is used once and the session lives in a
// cookie jar for the run. The session is short-lived, so the transport carries
// a refresh hook that trades a fresh token when Airflow stops accepting the
// one in hand.

// mwaaLoginPath is where an MWAA web server trades a web login token for a
// session cookie.
const mwaaLoginPath = "/aws_mwaa/login"

// openFallback trades a web login token for a session and builds the HTTP
// transport behind it. The refusal that sent us here travels with any failure:
// an environment where neither way in works should say so once, with both
// reasons, rather than hide the first behind the second.
//
// The lock is held across two network calls, which the exec helper and
// airflowapi.Client.detect both go out of their way not to do. It is tolerated
// here because a caller that arrives while the exchange is running wants
// exactly its result and has nothing useful to do with an early return: the
// AWS API has already refused it, so there is no second way to try. What it
// costs is a caller's deadline, which cannot end the wait — worth revisiting
// if anything ever fans out requests across one instance, which nothing does
// today.
func (t *awsTransport) openFallback(ctx context.Context, refused error) (airflowapi.Transport, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.fallback != nil {
		return t.fallback, nil
	}
	fallback, err := t.webLoginTransport(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s in %s refused the AWS API (%w) and the web login failed too: %w",
			t.environment, t.region, refused, err)
	}
	t.fallback = fallback
	return fallback, nil
}

// webLoginTransport walks the exchange above and hands back an ordinary HTTP
// transport carrying the session cookie it earned.
func (t *awsTransport) webLoginTransport(ctx context.Context) (airflowapi.Transport, error) {
	client, err := withCookieJar(t.base)
	if err != nil {
		return nil, err
	}
	session := &webSession{api: t.api, environment: t.environment, client: client}
	baseURL, err := session.open(ctx)
	if err != nil {
		return nil, err
	}
	// The refresh hook re-runs the whole exchange, because there is nothing
	// smaller to re-run: the token is one-use and the session it bought is what
	// expired. It fires on the 401 or 403 that expiry looks like, once, which
	// is the transport's only retry.
	return airflowapi.NewHTTPTransport(baseURL,
		airflowapi.WithHTTPClient(client),
		airflowapi.WithRefresh(session.refresh))
}

// webSession holds what it takes to earn an MWAA web session, so earning
// another one mid-command is one call rather than a rebuild.
type webSession struct {
	api         mwaaAPI
	environment string
	client      *http.Client
	// baseURL is where the session was earned. MWAA hands the hostname back
	// with the token, and a refresh asks again rather than assuming it stayed
	// the same.
	baseURL string
}

// open mints a web login token, trades it for a session cookie, and reports
// where that session is good.
func (s *webSession) open(ctx context.Context) (string, error) {
	out, err := s.api.CreateWebLoginToken(ctx, &mwaa.CreateWebLoginTokenInput{Name: aws.String(s.environment)})
	if err != nil {
		return "", fmt.Errorf("create a web login token for %s: %w", s.environment, err)
	}
	hostname := aws.ToString(out.WebServerHostname)
	token := aws.ToString(out.WebToken)
	if hostname == "" || token == "" {
		return "", fmt.Errorf("MWAA returned no web server hostname or token for %s", s.environment)
	}
	baseURL := "https://" + hostname
	if err := s.exchange(ctx, baseURL, token); err != nil {
		return "", err
	}
	s.baseURL = baseURL
	return baseURL, nil
}

func (s *webSession) refresh(ctx context.Context) error {
	renewed, err := s.open(ctx)
	if err != nil {
		return err
	}
	if renewed != s.baseURL {
		// Unreachable while MWAA keeps an environment on one web server, and
		// worth saying rather than silently talking to the old one: the
		// transport's base URL was fixed when it was built.
		return fmt.Errorf("%s moved to a different web server mid-command; run this again", s.environment)
	}
	return nil
}

// exchange posts the login token and lets the client's jar keep whatever
// session cookie comes back. The web server answers with a redirect into its
// own UI, so a redirect counts as success — a rejection comes back 4xx.
//
// The cookie is checked against the API's own URL rather than the login's. A
// Set-Cookie with no Path attribute is scoped to the directory it came from,
// so a session cookie set at /aws_mwaa/login without one is good for
// /aws_mwaa/ and nothing else: the login would look like it worked and every
// call after it would ride unauthenticated.
func (s *webSession) exchange(ctx context.Context, baseURL, token string) error {
	loginURL := baseURL + mwaaLoginPath
	form := url.Values{"token": {token}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("trade the web login token at %s: %w", loginURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("%s answered %d %s to the web login token", loginURL, resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	apiURL, err := url.Parse(baseURL + airflowapi.Airflow2.BasePath() + "/dags")
	if err != nil {
		return err
	}
	if len(s.client.Jar.Cookies(apiURL)) == 0 {
		return fmt.Errorf("%s accepted the web login token but its session cookie does not reach the Airflow API — it may be scoped to %s",
			loginURL, mwaaLoginPath)
	}
	return nil
}

// withCookieJar copies a client and gives the copy a jar, so the session the
// login hands back rides every later request while the caller's own client —
// its timeout, its TLS settings, its proxy — is left as it was.
//
// Redirects are refused on the copy whatever the original did. The exchange
// answers with a hop into the Airflow UI, and following it would download a
// page nobody reads; the cookie is already in the jar by then.
func withCookieJar(base *http.Client) (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("hold the MWAA web session: %w", err)
	}
	clone := *base
	clone.Jar = jar
	clone.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &clone, nil
}
