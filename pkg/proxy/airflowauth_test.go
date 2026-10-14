package proxy

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// airflowAuthBackend simulates an Airflow 3 api-server whose /auth/token mints
// the current token, and whose /page and /api require it: /page redirects to
// the login flow on a missing or stale bearer, /api answers a bare 401.
func airflowAuthBackend(t *testing.T, token *string) (ts *httptest.Server, port string) {
	t.Helper()
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"` + *token + `"}`))
		case "/page":
			if r.Header.Get("Authorization") != "Bearer "+*token {
				w.Header().Set("Location", "/auth/login?next=http%3A%2F%2Flocalhost%2Fpage")
				w.WriteHeader(http.StatusTemporaryRedirect)
				return
			}
			_, _ = w.Write([]byte("ok"))
		case "/api":
			if r.Header.Get("Authorization") != "Bearer "+*token {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte("ok"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	return ts, backendPortOf(t, ts)
}

// af2AuthBackend simulates an Airflow 2 webserver: there is no /auth/token
// (404 → fetchAirflowAuth falls through to the FAB login flow), GET /login
// serves a CSRF form, POST /login mints the current session cookie, and /page
// requires it. Mirroring Flask/Werkzeug, /page reads the FIRST `session` cookie
// when duplicates arrive — the semantics that make an appended fresh cookie
// useless while the stale one still rides in front. Rejections redirect with an
// absolute-URL Location, as FAB emits. pageCookies records the cookies each
// /page request carried, in order; logins counts successful POST /login.
func af2AuthBackend(t *testing.T, session string) (ts *httptest.Server, port string, pageCookies *[][]*http.Cookie, logins *int) {
	t.Helper()
	var mu sync.Mutex
	var seen [][]*http.Cookie
	var posts int
	pageCookies = &seen
	logins = &posts
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			if r.Method == http.MethodPost {
				if r.FormValue("csrf_token") != "csrf-123" || r.FormValue("username") != "admin" {
					http.Error(w, "bad login", http.StatusBadRequest)
					return
				}
				mu.Lock()
				posts++
				mu.Unlock()
				http.SetCookie(w, &http.Cookie{Name: airflowSessionCookie, Value: session, Path: "/"})
				_, _ = w.Write([]byte("logged in"))
				return
			}
			_, _ = w.Write([]byte(`<input name="csrf_token" value="csrf-123">`))
		case "/page":
			mu.Lock()
			seen = append(seen, r.Cookies())
			mu.Unlock()
			var first string
			for _, c := range r.Cookies() {
				if c.Name == airflowSessionCookie {
					first = c.Value
					break
				}
			}
			if first != session {
				w.Header().Set("Location", ts.URL+"/login/?next="+url.QueryEscape(ts.URL+"/page"))
				w.WriteHeader(http.StatusFound)
				return
			}
			_, _ = w.Write([]byte("ok"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	return ts, backendPortOf(t, ts), pageCookies, logins
}

func backendPortOf(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Port()
}

// proxiedGet sends GET path through p's reverse proxy for backendPort, as a
// browser would through a project's hostname, and returns the response.
func proxiedGet(t *testing.T, p *Proxy, backendPort, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	p.getOrCreateProxy(backendPort).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://project.localhost"+path, http.NoBody))
	return rec
}

func TestAuthRetryTransportRefreshesStaleSession(t *testing.T) {
	token := "fresh"
	ts, port := airflowAuthBackend(t, &token)

	s := NewAirflowSessions()
	s.sessions[port] = "Bearer stale" // as if cached before a restart

	rt := s.Transport(port, http.DefaultTransport)
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/page", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	injectAirflowAuth(req, s.get(port))

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 after refresh-and-retry", resp.StatusCode)
	}
	if got := s.get(port); got != "Bearer fresh" {
		t.Fatalf("cached session = %q, want refreshed bearer", got)
	}
}

func TestEnsureRetriesFailuresAfterTTL(t *testing.T) {
	token := "boot"
	_, port := airflowAuthBackend(t, &token)

	s := NewAirflowSessions()
	// As if the first fetch raced Airflow's boot and failed.
	s.sessions[port] = airflowAuthNone
	s.tried[port] = time.Now()

	s.Ensure(port)
	if got := s.get(port); got != airflowAuthNone {
		t.Fatalf("session = %q, want failure kept within TTL", got)
	}

	s.tried[port] = time.Now().Add(-2 * authRetryTTL)
	s.Ensure(port)
	if got := s.get(port); got != "Bearer boot" {
		t.Fatalf("session = %q, want re-fetched bearer after TTL", got)
	}
}

// The non-redirect branch of isAirflowAuthRejection: API endpoints reject with
// a bare 401 rather than a login redirect, and the transport must refresh and
// replay those too.
func TestAuthRetryTransportRecoversFromPlain401(t *testing.T) {
	token := "fresh"
	ts, port := airflowAuthBackend(t, &token)

	s := NewAirflowSessions()
	s.sessions[port] = "Bearer stale" // as if cached before a restart

	rt := s.Transport(port, http.DefaultTransport)
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	injectAirflowAuth(req, s.get(port))

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 after refresh-and-replay of a plain 401", resp.StatusCode)
	}
}

// The Airflow 2 half of the recovery pass: the replay must REPLACE the stale
// session cookie, not ride alongside it. Unless the retry strips the rejected
// cookie first, the replay carries "session=stale; session=fresh", Flask reads
// the first, and the user dead-ends at the login flow the recovery exists to
// avoid. Also exercises the absolute-URL Location form of the login redirect
// and preservation of an unrelated cookie across the rebuild.
func TestAuthRetryTransportRefreshesStaleAF2Cookie(t *testing.T) {
	ts, port, pageCookies, _ := af2AuthBackend(t, "fresh")

	s := NewAirflowSessions()
	s.sessions[port] = "Cookie session=stale" // as if cached before a restart

	rt := s.Transport(port, http.DefaultTransport)
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/page", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	// As the request leaves the Rewrite hook: the browser's unrelated cookie,
	// then injectAirflowAuth adding the (stale) cached session.
	req.AddCookie(&http.Cookie{Name: "X-Airflow-Port", Value: port})
	injectAirflowAuth(req, s.get(port))

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 after refresh-and-replay", resp.StatusCode)
	}
	if got := s.get(port); got != "Cookie session=fresh" {
		t.Fatalf("cached session = %q, want refreshed cookie", got)
	}

	seen := *pageCookies
	if len(seen) != 2 {
		t.Fatalf("page requests = %d, want original + exactly one replay", len(seen))
	}
	sessions, others := cookieValues(seen[1], airflowSessionCookie), cookieValues(seen[1], "X-Airflow-Port")
	if len(sessions) != 1 || sessions[0] != "fresh" {
		t.Fatalf("replay carried session cookies %v, want exactly [fresh] — the stale cookie must be stripped, not appended to", sessions)
	}
	if len(others) != 1 || others[0] != port {
		t.Fatalf("replay carried other cookies %v, want the unrelated cookie preserved", others)
	}
}

func cookieValues(cookies []*http.Cookie, name string) []string {
	var vals []string
	for _, c := range cookies {
		if c.Name == name {
			vals = append(vals, c.Value)
		}
	}
	return vals
}

// The replay guard: a request whose body can't be reproduced (GetBody == nil —
// which is every bodied request the ReverseProxy forwards) must not be
// replayed, and no session refresh should even be attempted; the original
// rejection goes back to the caller untouched.
func TestAuthRetryTransportNeverReplaysBodiedRequests(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(ts.Close)
	port := backendPortOf(t, ts)

	s := NewAirflowSessions()
	s.sessions[port] = "Bearer stale"

	rt := s.Transport(port, http.DefaultTransport)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v2/dags/x/dagRuns", strings.NewReader(`{"conf":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.GetBody = nil // as the ReverseProxy forwards it: the body streams and can't be reproduced
	injectAirflowAuth(req, s.get(port))

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want the original 401 passed through", resp.StatusCode)
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Fatalf("backend hits = %d, want 1 — a bodied request must not be replayed (extra hits mean a replay or an auth fetch happened)", hits)
	}
}

// The shape a real browser produces: Airflow 2's Set-Cookie responses pass
// through the proxy untouched, so the browser holds its OWN session cookie and
// sends it — riding ahead of anything the Rewrite hook adds.
//
// That ordering is load-bearing. requestAirflowAuth reads the request back with
// req.Cookie, which returns the FIRST match, so if injection merely appended,
// invalidation would be keyed on the browser's value, never match the cache,
// and Ensure would keep serving the stale credential forever — cached successes
// have no TTL, so the recovery this transport exists to provide would silently
// never fire.
func TestAuthRetryTransportRecoversWithBrowserHeldSessionCookie(t *testing.T) {
	ts, port, pageCookies, _ := af2AuthBackend(t, "fresh")

	s := NewAirflowSessions()
	s.sessions[port] = "Cookie session=stale-cached" // as if cached before a restart

	rt := s.Transport(port, http.DefaultTransport)
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/page", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	// The browser's own cookies go on first, exactly as they arrive, and only
	// then does the Rewrite hook apply the cached auth.
	req.AddCookie(&http.Cookie{Name: "X-Airflow-Port", Value: port})
	req.AddCookie(&http.Cookie{Name: airflowSessionCookie, Value: "stale-browser"})
	injectAirflowAuth(req, s.get(port))

	// The outbound request must carry our credential, not the browser's.
	if got := requestAirflowAuth(req); got != "Cookie session=stale-cached" {
		t.Fatalf("requestAirflowAuth = %q, want the injected credential — invalidation keys on this", got)
	}

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 after refresh-and-replay", resp.StatusCode)
	}
	if got := s.get(port); got != "Cookie session=fresh" {
		t.Fatalf("cached session = %q, want the cache to have moved on to the refreshed cookie", got)
	}

	seen := *pageCookies
	if len(seen) != 2 {
		t.Fatalf("page requests = %d, want original + exactly one replay", len(seen))
	}
	sessions, others := cookieValues(seen[1], airflowSessionCookie), cookieValues(seen[1], "X-Airflow-Port")
	if len(sessions) != 1 || sessions[0] != "fresh" {
		t.Fatalf("replay carried session cookies %v, want exactly [fresh] — both the browser's and the rejected cookie must be gone", sessions)
	}
	if len(others) != 1 || others[0] != port {
		t.Fatalf("replay carried other cookies %v, want the unrelated cookie preserved", others)
	}
}

// The conservative half of the replace rule: with nothing to offer, the
// caller's own session cookie must survive rather than being stripped for
// nothing.
func TestInjectAirflowAuthLeavesCookiesAloneWithoutCredential(t *testing.T) {
	for _, auth := range []string{"", airflowAuthNone} {
		req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/page", http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(&http.Cookie{Name: airflowSessionCookie, Value: "browser"})
		injectAirflowAuth(req, auth)
		c, err := req.Cookie(airflowSessionCookie)
		if err != nil || c.Value != "browser" {
			t.Errorf("auth %q: session cookie = %v (err %v), want the browser's own preserved", auth, c, err)
		}
	}
}

// isAirflowAuthRejection parses Location and compares only the path, and the
// doc comment leans on that to rule out false positives. Every other test here
// feeds it a true positive, so a "simplification" to a substring check would
// keep the suite green while breaking unrelated redirects — a /login-help page
// or a ?next=/auth/login query would start triggering pointless re-auth and
// replay.
func TestIsAirflowAuthRejection(t *testing.T) {
	// status + Location pairs rather than prebuilt responses: these are
	// synthetic headers with no body, and a helper returning *http.Response
	// trips bodyclose.
	type probe struct {
		status   int
		location string
	}

	rejections := map[string]probe{
		"401":                   {http.StatusUnauthorized, ""},
		"relative AF3 login":    {http.StatusTemporaryRedirect, "/auth/login"},
		"relative AF2 login":    {http.StatusFound, "/login"},
		"trailing slash":        {http.StatusFound, "/login/"},
		"absolute FAB login":    {http.StatusFound, "http://localhost:8080/login/?next=%2Fhome"},
		"login with next query": {http.StatusFound, "/auth/login?next=%2Fdags"},
	}
	notRejections := map[string]probe{
		"ok":                     {http.StatusOK, ""},
		"server error":           {http.StatusInternalServerError, ""},
		"forbidden is not stale": {http.StatusForbidden, ""},
		"not modified":           {http.StatusNotModified, ""},
		"redirect, no Location":  {http.StatusFound, ""},
		"login-help page":        {http.StatusFound, "/login-help"},
		"login as a path prefix": {http.StatusFound, "/loginsomething"},
		"login deeper in path":   {http.StatusFound, "/docs/login/guide"},
		"next names login":       {http.StatusFound, "/home?next=/auth/login"},
		"absolute non-login":     {http.StatusFound, "http://localhost:8080/home"},
	}

	check := func(t *testing.T, cases map[string]probe, want bool) {
		t.Helper()
		for name, c := range cases {
			r := &http.Response{StatusCode: c.status, Header: make(http.Header)}
			if c.location != "" {
				r.Header.Set("Location", c.location)
			}
			if got := isAirflowAuthRejection(r); got != want {
				t.Errorf("%s: isAirflowAuthRejection = %v, want %v", name, got, want)
			}
		}
	}

	t.Run("rejections", func(t *testing.T) { check(t, rejections, true) })
	t.Run("not rejections", func(t *testing.T) { check(t, notRejections, false) })
}

// A page load rejects many requests at once. Without collapsing them, each
// runs its own login flow — for Airflow 2 a full two-request FAB login apiece.
func TestEnsureCollapsesConcurrentLogins(t *testing.T) {
	var mu sync.Mutex
	logins := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			mu.Lock()
			logins++
			mu.Unlock()
			time.Sleep(50 * time.Millisecond) // hold the flow open so callers pile up
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"tok"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(ts.Close)
	port := strconv.Itoa(ts.Listener.Addr().(*net.TCPAddr).Port)

	s := NewAirflowSessions()
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() { s.Ensure(port) })
	}
	wg.Wait()

	mu.Lock()
	got := logins
	mu.Unlock()
	if got != 1 {
		t.Errorf("login flows = %d, want exactly 1 for a concurrent burst", got)
	}
	if auth := s.get(port); auth != "Bearer tok" {
		t.Errorf("cached session = %q, want the fetched bearer", auth)
	}
}

// A bodied request can't be replayed, but the credential it carried is still
// known-bad. Leaving it cached means the user's next request fails the same
// way; dropping it means the retry (or the page's next GET) re-authenticates
// and succeeds. The drop must not itself trigger a login — that's the replay
// guard's job, and the request has already returned.
func TestAuthRetryTransportInvalidatesOnBodiedRejection(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(ts.Close)
	port := backendPortOf(t, ts)

	s := NewAirflowSessions()
	s.sessions[port] = "Bearer stale"
	s.tried[port] = time.Now()

	rt := s.Transport(port, http.DefaultTransport)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v2/dags/x/dagRuns", strings.NewReader(`{"conf":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.GetBody = nil
	injectAirflowAuth(req, s.get(port))

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if _, cached := s.sessions[port]; cached {
		t.Error("the rejected credential should have been dropped from the cache")
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Errorf("backend hits = %d, want 1 — invalidation must not replay or re-authenticate", hits)
	}
}

// The point of the move: a page opened through the proxy for an Airflow 2
// project arrives logged in, the proxy having run FAB's login form itself.
func TestAProxiedRequestArrivesLoggedInToAirflow2(t *testing.T) {
	_, port, pageCookies, _ := af2AuthBackend(t, "minted")
	p := NewProxy("0", NewStore(t.TempDir()))

	rec := proxiedGet(t, p, port, "/page")
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("status %d, body %q: want Airflow 2's page, not its login", rec.Code, rec.Body.String())
	}
	if seen := *pageCookies; len(seen) != 1 || len(cookieValues(seen[0], airflowSessionCookie)) != 1 {
		t.Fatalf("page requests %v, want one, carrying the minted session", seen)
	}
}

// An Airflow restart invalidates the cached session; the next page through the
// proxy still arrives, because the proxy's transport carries the recovery pass.
func TestAProxiedRequestRecoversFromAStaleSession(t *testing.T) {
	_, port, pageCookies, _ := af2AuthBackend(t, "after-restart")
	p := NewProxy("0", NewStore(t.TempDir()))
	p.Sessions.sessions[port] = "Cookie session=before-restart"
	p.Sessions.tried[port] = time.Now()

	rec := proxiedGet(t, p, port, "/page")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want the page after a refresh-and-replay", rec.Code)
	}
	if n := len(*pageCookies); n != 2 {
		t.Fatalf("page requests = %d, want the rejected one and its replay", n)
	}
}

// A local Airflow 3 runs with every user an admin, so the proxy's login is a
// formality: its token request answers at once and every page goes through as
// it would without the proxy. Once per backend, not per request.
func TestAnAirflow3WithAllAdminsIsUnaffected(t *testing.T) {
	var mu sync.Mutex
	tokens := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			mu.Lock()
			tokens++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"access_token":"anyone-is-admin"}`))
		default:
			_, _ = w.Write([]byte("page " + r.URL.Path))
		}
	}))
	t.Cleanup(ts.Close)
	port := backendPortOf(t, ts)
	p := NewProxy("0", NewStore(t.TempDir()))

	for _, path := range []string{"/", "/dags", "/api/v2/dags"} {
		if rec := proxiedGet(t, p, port, path); rec.Code != http.StatusOK || rec.Body.String() != "page "+path {
			t.Errorf("%s: status %d, body %q, want the page unchanged", path, rec.Code, rec.Body.String())
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if tokens != 1 {
		t.Errorf("token requests = %d, want 1 for the backend, not one per page", tokens)
	}
}

// A backend that offers no login at all still answers every page. The failed
// login is cached, so it costs one attempt per authRetryTTL rather than one per
// request.
func TestABackendWithNoLoginIsLeftAlone(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token", "/login":
			mu.Lock()
			attempts++
			mu.Unlock()
			http.NotFound(w, r)
		default:
			_, _ = w.Write([]byte("open"))
		}
	}))
	t.Cleanup(ts.Close)
	port := backendPortOf(t, ts)
	p := NewProxy("0", NewStore(t.TempDir()))

	for range 3 {
		if rec := proxiedGet(t, p, port, "/page"); rec.Code != http.StatusOK || rec.Body.String() != "open" {
			t.Fatalf("status %d, body %q, want the page", rec.Code, rec.Body.String())
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if attempts != 2 { // one /auth/token, one GET /login, then the cached failure
		t.Errorf("login requests = %d, want 2 — one attempt, then the failure cached", attempts)
	}
}

// Sessions nil turns the login off: the backend sees no login attempt and the
// page answers as Airflow would to anyone.
func TestNilSessionsTurnsTheLoginOff(t *testing.T) {
	_, port, _, logins := af2AuthBackend(t, "minted")
	p := NewProxy("0", NewStore(t.TempDir()))
	p.Sessions = nil

	if rec := proxiedGet(t, p, port, "/page"); rec.Code != http.StatusFound {
		t.Errorf("status %d, want Airflow 2's redirect to its login page", rec.Code)
	}
	if *logins != 0 {
		t.Errorf("logins = %d, want none", *logins)
	}
}

// One cache shared by a host's proxies logs each backend in once, however many
// of them relay it. The desktop's hostname proxy and its iframe proxies are
// the case this exists for.
func TestASharedSessionLogsInOnce(t *testing.T) {
	_, port, _, logins := af2AuthBackend(t, "minted")
	shared := NewAirflowSessions()
	a := NewProxy("0", NewStore(t.TempDir()))
	b := NewProxy("0", NewStore(t.TempDir()))
	a.Sessions, b.Sessions = shared, shared

	for _, p := range []*Proxy{a, b} {
		if rec := proxiedGet(t, p, port, "/page"); rec.Code != http.StatusOK {
			t.Fatalf("status %d, want the page", rec.Code)
		}
	}
	if *logins != 1 {
		t.Errorf("logins = %d, want 1 across both proxies", *logins)
	}
}

// The session's recovery pass wraps the host's transport rather than replacing
// it, so a host's settings (the desktop turns keep-alives off) carry every
// round trip, the replay included.
func TestTheSessionWrapsTheHostsTransport(t *testing.T) {
	host := &http.Transport{DisableKeepAlives: true}
	p := NewProxy("0", NewStore(t.TempDir()))
	p.Transport = func(string) http.RoundTripper { return host }

	retry, ok := p.transportFor("15001").(*authRetryTransport)
	if !ok {
		t.Fatalf("transport = %T, want the session's recovery pass", p.transportFor("15001"))
	}
	if retry.inner != host {
		t.Errorf("the recovery pass wraps %T, want the host's transport", retry.inner)
	}

	p.Sessions = nil
	if got := p.transportFor("15001"); got != host {
		t.Errorf("with no Sessions, transport = %T, want the host's own", got)
	}
}
