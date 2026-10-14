package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
)

// airflowAuthNone is a sentinel stored in the session cache when auth has been
// attempted and failed, preventing repeated retries on every request.
const airflowAuthNone = "-"

// airflowSessionCookie is the name of Airflow 2's FAB session cookie — the one
// fetchAirflowAuth captures and injectAirflowAuth (re)injects.
const airflowSessionCookie = "session"

// authRetryTTL is how long a cached auth *failure* suppresses re-fetching.
// Short on purpose: the common failure is racing Airflow's boot (a tool opens
// Airflow within seconds of a start), which resolves as soon as the api-server
// is up. Tests exercise expiry by backdating the tried time rather than
// shrinking this.
const authRetryTTL = 2 * time.Second

const (
	// authTokenTimeout bounds Airflow 3's token request, and authLoginTimeout
	// each request of Airflow 2's login flow. Both run on the request path of
	// the first request to a backend, so a wedged backend costs at most this.
	authTokenTimeout = 5 * time.Second
	authLoginTimeout = 10 * time.Second
)

// AirflowSessions logs a proxy into the local Airflows behind it, so a person
// opening a project in a browser lands in Airflow rather than at its login page.
//
// One session per backend port, fetched on first use with the local admin
// account every engine creates (pkg/airflowrt's Airflow2AdminUser and
// Airflow2AdminPassword):
//
//   - Airflow 3: a bearer token from /auth/token. Under
//     simple_auth_manager_all_admins, which every local engine sets, that
//     endpoint hands out an admin token without checking the credentials, so
//     this is one fast round trip and changes nothing about who may do what.
//   - Airflow 2: the session cookie from Flask-AppBuilder's login form.
//
// The zero value is not usable; use NewAirflowSessions. Safe for concurrent
// use, and meant to be shared: a host that relays the same backends through
// more than one proxy (the desktop's hostname proxy and its per-project iframe
// proxies) passes one instance to all of them, so each backend is logged into
// once per process.
type AirflowSessions struct {
	mu       sync.RWMutex
	sessions map[string]string    // backend port → "Bearer …" / "Cookie session=…" / airflowAuthNone
	tried    map[string]time.Time // backend port → when the cached value was fetched
	// In-flight fetches keyed by backend port. A page load fires many parallel
	// requests, so a restart rejects them all at once; without this every one
	// of them would run its own login flow (for Airflow 2, a full two-request
	// FAB login, up to ~20s each against a wedged backend). The first caller
	// fetches, the rest wait on its channel and use the result.
	inflight map[string]chan struct{}

	// fetch obtains a fresh credential for a backend port. A seam for tests.
	fetch func(backendPort string) (string, error)
}

// NewAirflowSessions returns an empty session cache.
func NewAirflowSessions() *AirflowSessions {
	return &AirflowSessions{
		sessions: make(map[string]string),
		tried:    make(map[string]time.Time),
		inflight: make(map[string]chan struct{}),
		fetch:    fetchAirflowAuth,
	}
}

// Ensure fetches and caches a session for backendPort if none is cached. A
// cached failure is retried once authRetryTTL has passed.
//
// Blocking: the first call for a backend makes the login round trips. A host
// can call it ahead of time to warm the cache, so a person's first request
// doesn't pay for them.
func (s *AirflowSessions) Ensure(backendPort string) {
	s.mu.Lock()
	auth, ok := s.sessions[backendPort]
	tried := s.tried[backendPort]
	if ok && (auth != airflowAuthNone || time.Since(tried) < authRetryTTL) {
		s.mu.Unlock()
		return
	}
	// Someone is already logging in for this port — wait for their result
	// instead of starting a competing login flow.
	if wait, busy := s.inflight[backendPort]; busy {
		s.mu.Unlock()
		<-wait
		return
	}
	done := make(chan struct{})
	s.inflight[backendPort] = done
	s.mu.Unlock()
	// Waiters block until this fetch has been cached, so the channel must be
	// closed on every exit path.
	defer func() {
		s.mu.Lock()
		delete(s.inflight, backendPort)
		s.mu.Unlock()
		close(done)
	}()

	fetched, err := s.fetch(backendPort)
	if err != nil {
		// Debug, not Error: the common causes are racing Airflow's boot, which
		// clears within authRetryTTL, and an Airflow 2 whose admin password is
		// not the local default (a non-macOS standalone generates one), which
		// FAB refuses for as long as the project runs and would otherwise log on
		// every retry while someone browses it. Either way the request goes
		// through with nothing of ours on it, the person's own session cookie
		// included, and Airflow answers it as it would anyone.
		slog.Debug("proxy: could not log in to Airflow", "backendPort", backendPort, "error", err)
		fetched = airflowAuthNone // cache the failure; retried after authRetryTTL
	}

	s.mu.Lock()
	// Double-check under the write lock: an invalidate can land while the
	// fetch is in flight (it's a network call, deliberately outside the lock),
	// and a fetch that failed must not clobber a good credential a faster
	// sibling just cached.
	if fetched != airflowAuthNone {
		s.sessions[backendPort] = fetched
		s.tried[backendPort] = time.Now()
	} else if existing, cached := s.sessions[backendPort]; !cached || existing == airflowAuthNone {
		s.sessions[backendPort] = fetched
		s.tried[backendPort] = time.Now()
	}
	s.mu.Unlock()
}

// Inject makes sure there is a session for backendPort and puts it on req.
// With no session to offer — the login failed, or the backend needs none —
// req is left as it came, so the caller's own cookie gets its chance.
func (s *AirflowSessions) Inject(req *http.Request, backendPort string) {
	s.Ensure(backendPort)
	injectAirflowAuth(req, s.get(backendPort))
}

// Transport wraps inner with one recovery pass for backendPort: when Airflow
// rejects a request's credential, drop it from the cache, log in again, and
// replay the request once with the fresh one. Covers both stale credentials (an
// Airflow restart mints a new signing key, so old bearers stop working) and the
// boot race (the first login failed because the api-server wasn't up yet).
//
// inner does the actual round trips, replay included, so a host's transport
// settings (keep-alives, timeouts) apply to both.
func (s *AirflowSessions) Transport(backendPort string, inner http.RoundTripper) http.RoundTripper {
	return &authRetryTransport{s: s, backendPort: backendPort, inner: inner}
}

// get returns the cached auth value for a backend port.
func (s *AirflowSessions) get(backendPort string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[backendPort]
}

// invalidateIf drops the cached session for a port so the next request
// re-authenticates — used when the backend rejects the cached credential. The
// drop only happens while the cache still holds the rejected credential; if a
// concurrent rejection already refreshed it, the caller should reuse that
// instead of forcing another login flow.
func (s *AirflowSessions) invalidateIf(backendPort, rejected string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions[backendPort] != rejected {
		return
	}
	delete(s.sessions, backendPort)
	delete(s.tried, backendPort)
}

// authRetryTransport is AirflowSessions.Transport's RoundTripper.
type authRetryTransport struct {
	s           *AirflowSessions
	backendPort string
	inner       http.RoundTripper
}

func (t *authRetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.inner.RoundTrip(req)
	if err != nil || !isAirflowAuthRejection(resp) {
		return resp, err
	}
	// Invalidate keyed on the credential THIS request carried, so a burst of
	// rejections from one page load doesn't each force their own login: the
	// first drops the stale entry, the rest see the cache has already moved on
	// (and Ensure collapses any genuine overlap).
	//
	// This runs before the replay guard below on purpose. A bodied request
	// can't be replayed, but leaving its credential cached is exactly what makes
	// the NEXT request fail too — dropping the rejected credential here means
	// the user's retry works instead of hitting the same wall.
	rejected := requestAirflowAuth(req)
	t.s.invalidateIf(t.backendPort, rejected)

	// Only replay requests whose body can be reproduced (page and API GETs are
	// the ones that matter — they're what a page issues on load). http.NoBody
	// is an empty body, so there is nothing to reproduce.
	if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		return resp, nil
	}

	t.s.Ensure(t.backendPort)
	fresh := t.s.get(t.backendPort)
	if fresh == "" || fresh == airflowAuthNone || fresh == rejected {
		return resp, nil
	}

	retry := req.Clone(req.Context())
	if req.GetBody != nil {
		body, bodyErr := req.GetBody()
		if bodyErr != nil {
			return resp, nil //nolint:nilerr // fall back to the original rejection
		}
		retry.Body = body
	}
	// Drop the rejected bearer before re-injecting; injectAirflowAuth Sets the
	// Authorization header so this is belt-and-braces, and it replaces the
	// Airflow 2 session cookie itself.
	retry.Header.Del("Authorization")
	injectAirflowAuth(retry, fresh)

	_ = resp.Body.Close()
	slog.Info("proxy: replaying request with a refreshed Airflow session", "backendPort", t.backendPort)
	return t.inner.RoundTrip(retry)
}

// injectAirflowAuth applies a cached auth value to an outbound request. auth is
// "Bearer <token>" for Airflow 3, "Cookie session=<value>" for Airflow 2,
// airflowAuthNone or "" when auth is unavailable.
//
// The Airflow 2 session cookie REPLACES any the caller already carried.
// Airflow 2's Set-Cookie responses pass through the proxy untouched, so the
// browser holds its own copy of whatever session was last minted — and
// appending would send "session=<browser>; session=<ours>", where Flask reads
// the first. That has two costs: the browser's stale cookie shadows the fresh
// one we just fetched, and requestAirflowAuth (which reads the request back to
// decide whether the cache still holds the credential the backend rejected)
// would key invalidation on the browser's value, so it would never match the
// cache and recovery after an Airflow restart would never fire.
//
// Nothing is stripped when auth is unavailable ("" / airflowAuthNone): we have
// nothing better to offer, so the caller's own cookie gets its chance.
func injectAirflowAuth(req *http.Request, auth string) {
	switch {
	case strings.HasPrefix(auth, "Bearer "):
		req.Header.Set("Authorization", auth)
	case strings.HasPrefix(auth, "Cookie "):
		cookie := strings.TrimPrefix(auth, "Cookie ")
		stripCookie(req, airflowSessionCookie)
		if existing := req.Header.Get("Cookie"); existing != "" {
			req.Header.Set("Cookie", existing+"; "+cookie)
		} else {
			req.Header.Set("Cookie", cookie)
		}
	}
}

// requestAirflowAuth reconstructs the cache-format auth string
// ("Bearer <tok>" / "Cookie session=<v>") that injectAirflowAuth applied to
// req, so the retry path can tell whether the cache still holds the credential
// the backend just rejected.
func requestAirflowAuth(req *http.Request) string {
	if auth := req.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return auth
	}
	if c, err := req.Cookie(airflowSessionCookie); err == nil {
		return "Cookie " + airflowSessionCookie + "=" + c.Value
	}
	return ""
}

// stripCookie removes the named cookie from req's Cookie header, preserving
// the rest.
func stripCookie(req *http.Request, name string) {
	cookies := req.Cookies()
	req.Header.Del("Cookie")
	for _, c := range cookies {
		if c.Name != name {
			req.AddCookie(c)
		}
	}
}

// isAirflowAuthRejection reports whether a backend response means "your
// credentials are missing or stale": a 401, or a redirect into the login flow
// (Airflow 3's /auth/login, Airflow 2's /login). Left unhandled, the browser
// follows that redirect and Airflow rejects the proxied `next` URL with
// "Invalid or unsafe next URL" — the login flow only trusts its own origin.
//
// The Location header is parsed and only its path compared: FAB emits
// absolute-URL Locations (http://host/login/?next=…), and substring checks
// would false-positive on an unencoded ?next=/auth/login query or unrelated
// paths like /login-help. A genuine logout→login redirect still matches, which
// costs at most one auth fetch and one replay — bounded, never a loop.
func isAirflowAuthRejection(resp *http.Response) bool {
	if resp.StatusCode == http.StatusUnauthorized {
		return true
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		loc, err := url.Parse(resp.Header.Get("Location"))
		if err != nil {
			return false
		}
		path := strings.TrimSuffix(loc.Path, "/")
		return path == "/login" || path == "/auth/login"
	}
	return false
}

// adminLoginForm is the local admin account as a login form body, the shape
// both Airflow 3's /auth/token and Airflow 2's FAB login accept.
func adminLoginForm() url.Values {
	return url.Values{
		"username": {airflowrt.Airflow2AdminUser},
		"password": {airflowrt.Airflow2AdminPassword},
	}
}

// fetchAirflowAuth logs in to the local Airflow on backendPort with the local
// admin account. It tries Airflow 3's /auth/token first, then falls back to
// Airflow 2's FAB session login flow, and returns "Bearer <token>" or
// "Cookie session=<value>" for whichever worked.
func fetchAirflowAuth(backendPort string) (string, error) {
	baseURL := "http://127.0.0.1:" + backendPort

	// --- Airflow 3: POST /auth/token ---
	if bearer, ok := fetchAirflow3Token(baseURL); ok {
		return bearer, nil
	}

	// --- Airflow 2: FAB session login flow ---
	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", fmt.Errorf("cookiejar: %w", err)
	}
	af2Client := &http.Client{Timeout: authLoginTimeout, Jar: jar}

	// Step 1: GET /login to obtain a session cookie and CSRF token.
	loginURL := baseURL + "/login"
	resp, err := af2Client.Get(loginURL) //nolint:noctx // af2Client's timeout bounds it
	if err != nil {
		return "", fmt.Errorf("airflow 2 GET /login: %w", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return "", fmt.Errorf("airflow 2 GET /login: %w", err)
	}

	csrfToken := extractFormCSRF(string(body))
	if csrfToken == "" {
		return "", fmt.Errorf("airflow 2: no CSRF token on the login page")
	}

	// Step 2: POST /login with the credentials and the CSRF token. Airflow 2's
	// login checks a real user row, seeded by whichever engine started this
	// Airflow from the same airflowrt constants.
	//
	// Not followed, because the redirect is the verdict. FAB answers a good
	// login with a redirect into the app and a bad one with a redirect back to
	// /login (a rejected form re-renders the page with a 200). The jar can't
	// tell them apart: Flask-WTF already put a session cookie in it on the GET
	// above, to hold the CSRF token, so "a session cookie is there" is true of
	// an anonymous session too. Caching that one as the login would replace a
	// person's own logged-in cookie with it on every request, and send each
	// request through a fresh login and a replay that fails the same way.
	af2Client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	form := adminLoginForm()
	form.Set("csrf_token", csrfToken)
	resp2, err := af2Client.PostForm(loginURL, form) //nolint:noctx // af2Client's timeout bounds it
	if err != nil {
		return "", fmt.Errorf("airflow 2 POST /login: %w", err)
	}
	_ = resp2.Body.Close()
	if !isLoggedInRedirect(resp2) {
		return "", fmt.Errorf("airflow 2: the local admin login was refused (status %d)", resp2.StatusCode)
	}

	// Step 3: Extract the session cookie from the jar, which keys cookies by
	// the URL they were set for. The jar takes the redirect's Set-Cookie even
	// though the redirect itself is not followed.
	for _, c := range jar.Cookies(&url.URL{Scheme: "http", Host: "127.0.0.1:" + backendPort}) {
		if c.Name == airflowSessionCookie {
			return "Cookie " + airflowSessionCookie + "=" + c.Value, nil
		}
	}
	return "", fmt.Errorf("airflow 2: no session cookie after login")
}

// isLoggedInRedirect reports whether FAB's answer to a login POST is a
// successful login: a redirect anywhere but back into the login flow.
func isLoggedInRedirect(resp *http.Response) bool {
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		return false
	}
	return !isAirflowAuthRejection(resp)
}

// fetchAirflow3Token asks Airflow 3's /auth/token for a bearer. False means try
// Airflow 2's login instead: anything but a token — a 404 from an Airflow 2, a
// refused connection, a body without one — lands there.
func fetchAirflow3Token(baseURL string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), authTokenTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/auth/token", strings.NewReader(adminLoginForm().Encode()))
	if err != nil {
		return "", false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", false
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil || tok.AccessToken == "" {
		return "", false
	}
	return "Bearer " + tok.AccessToken, true
}

// csrfRe pulls the CSRF token value out of a FAB / Flask-WTF login form, with
// the name and value attributes in either order.
var csrfRe = regexp.MustCompile(`(?i)name=["']csrf_token["'][^>]*value=["']([^"']+)["']|value=["']([^"']+)["'][^>]*name=["']csrf_token["']`)

func extractFormCSRF(html string) string {
	m := csrfRe.FindStringSubmatch(html)
	if len(m) > 1 {
		if m[1] != "" {
			return m[1]
		}
		return m[2]
	}
	return ""
}
