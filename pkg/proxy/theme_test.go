package proxy

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

const themeTag = `<style id="astro-theme">`

func newHTMLResponse(t *testing.T, body string, gzipped bool) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if gzipped {
		gw := gzip.NewWriter(&buf)
		_, _ = gw.Write([]byte(body))
		_ = gw.Close()
	} else {
		buf.WriteString(body)
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(buf.Bytes())),
	}
	resp.Header.Set("Content-Type", "text/html; charset=utf-8")
	resp.ContentLength = int64(buf.Len())
	if gzipped {
		resp.Header.Set("Content-Encoding", "gzip")
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			t.Fatalf("gzip reader: %v", err)
		}
		out, err := io.ReadAll(gr)
		if err != nil {
			t.Fatalf("gzip read: %v", err)
		}
		_ = gr.Close()
		return string(out)
	}
	return string(body)
}

func TestInjectIntoHTMLBasicInjection(t *testing.T) {
	resp := newHTMLResponse(t, "<html><head><title>x</title></head><body></body></html>", false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	if err := InjectIntoHTML(resp, "<style>foo</style>", "</head>"); err != nil {
		t.Fatalf("inject: %v", err)
	}
	got := readBody(t, resp)
	want := "<html><head><title>x</title><style>foo</style></head><body></body></html>"
	if got != want {
		t.Fatalf("body mismatch:\n got: %q\nwant: %q", got, want)
	}
	if resp.ContentLength != int64(len(want)) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, len(want))
	}
	if cl := resp.Header.Get("Content-Length"); cl != strconv.Itoa(len(want)) {
		t.Errorf("Content-Length header = %q, want %q", cl, strconv.Itoa(len(want)))
	}
}

func TestInjectIntoHTMLNonHTMLPassesThrough(t *testing.T) {
	const original = "not actually html, but body says so"
	resp := newHTMLResponse(t, original, false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	resp.Header.Set("Content-Type", "application/json")

	if err := InjectIntoHTML(resp, "<style>x</style>", "</head>"); err != nil {
		t.Fatalf("inject: %v", err)
	}
	if got := readBody(t, resp); got != original {
		t.Errorf("non-HTML body was modified: got %q, want %q", got, original)
	}
}

func TestInjectIntoHTMLNoMarkerIsNoOpOnContent(t *testing.T) {
	body := "<html><body>no head tag here</body></html>"
	resp := newHTMLResponse(t, body, false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	if err := InjectIntoHTML(resp, "<style>x</style>", "</head>"); err != nil {
		t.Fatalf("inject: %v", err)
	}
	if got := readBody(t, resp); got != body {
		t.Errorf("body changed when marker absent: got %q, want %q", got, body)
	}
}

// Two </head> tags: the snippet goes before the LAST one, so an inline script
// that mentions "</head>" in a string doesn't catch the injection mid-document.
func TestInjectIntoHTMLUsesLastMarker(t *testing.T) {
	body := "<html>fake </head> in script<head><title>t</title></head><body></body></html>"
	resp := newHTMLResponse(t, body, false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	if err := InjectIntoHTML(resp, "<style>x</style>", "</head>"); err != nil {
		t.Fatalf("inject: %v", err)
	}
	got := readBody(t, resp)
	last := strings.LastIndex(got, "</head>")
	style := strings.LastIndex(got, "<style>x</style>")
	if style < 0 || last < 0 || style >= last {
		t.Errorf("snippet not placed before last </head>: got %q", got)
	}
	if n := strings.Count(got, "<style>x</style>"); n != 1 {
		t.Errorf("snippet appeared %d times, want 1: %q", n, got)
	}
}

func TestInjectIntoHTMLCaseInsensitiveMarker(t *testing.T) {
	resp := newHTMLResponse(t, "<HTML><HEAD></HEAD><BODY></BODY></HTML>", false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	if err := InjectIntoHTML(resp, "<style>x</style>", "</head>"); err != nil {
		t.Fatalf("inject: %v", err)
	}
	if got := readBody(t, resp); !strings.Contains(got, "<style>x</style></HEAD>") {
		t.Errorf("expected snippet before uppercase </HEAD>: got %q", got)
	}
}

func TestInjectIntoHTMLGzipRoundTrip(t *testing.T) {
	resp := newHTMLResponse(t, "<html><head></head><body></body></html>", true) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	if err := InjectIntoHTML(resp, "<style>x</style>", "</head>"); err != nil {
		t.Fatalf("inject: %v", err)
	}
	if resp.Header.Get("Content-Encoding") != "gzip" {
		t.Errorf("Content-Encoding lost on gzip body: got %q", resp.Header.Get("Content-Encoding"))
	}
	want := "<html><head><style>x</style></head><body></body></html>"
	if got := readBody(t, resp); got != want {
		t.Errorf("decoded body mismatch:\n got: %q\nwant: %q", got, want)
	}
}

// A non-gzip HTML response carrying a Content-Encoding header (a misconfigured
// upstream) is rewritten as plain bytes, so the header is cleared or a client
// would try to decode them.
func TestInjectIntoHTMLPlaintextStripsContentEncoding(t *testing.T) {
	resp := newHTMLResponse(t, "<html><head></head></html>", false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	resp.Header.Set("Content-Encoding", "identity")
	if err := InjectIntoHTML(resp, "<style>x</style>", "</head>"); err != nil {
		t.Fatalf("inject: %v", err)
	}
	if got := resp.Header.Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding not cleared on plaintext body: got %q", got)
	}
}

// A body that claims gzip but isn't: no error, so the response still reaches
// the client.
func TestInjectIntoHTMLBadGzipPassesThrough(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("not gzip data")),
	}
	resp.Header.Set("Content-Type", "text/html")
	resp.Header.Set("Content-Encoding", "gzip")
	if err := InjectIntoHTML(resp, "<style>x</style>", "</head>"); err != nil {
		t.Errorf("expected no error on bad gzip, got %v", err)
	}
}

func TestInjectAirflowThemeCarriesThePrePaintBackground(t *testing.T) {
	resp := newHTMLResponse(t, `<html><head><title>Airflow</title></head><body><div id="root"></div></body></html>`, false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	if err := InjectAirflowTheme(resp); err != nil {
		t.Fatalf("inject: %v", err)
	}
	got := readBody(t, resp)

	head := strings.Index(got, "</head>")
	script := strings.Index(got, "prefers-color-scheme: dark")
	if script < 0 || script > head {
		t.Errorf("pre-paint script not injected inside <head>:\n%s", got)
	}
	// Both ways the rule has to stop applying: the selector for Airflow 3,
	// which marks the root as next-themes boots, and the #root test for
	// Airflow 2, which never marks it and never needed the bridge.
	if !strings.Contains(got, "html:not(.light):not(.dark){background:") {
		t.Errorf("pre-paint rule is not scoped to the pre-boot root:\n%s", got)
	}
	if !strings.Contains(got, "if (!document.getElementById('root')) s.remove();") {
		t.Errorf("pre-paint rule never removes itself on a page with no #root:\n%s", got)
	}
	for _, want := range []string{"#161d2d", "#ffffff"} {
		if !strings.Contains(got, want) {
			t.Errorf("pre-paint script missing background %s:\n%s", want, got)
		}
	}
	// Shares the pass with the theme CSS rather than costing a second one.
	head = strings.Index(got, "</head>")
	if i := strings.Index(got, themeTag); i < 0 || i > head {
		t.Errorf("theme CSS is not in the head:\n%s", got)
	}
	if !strings.Contains(got, "--chakra-colors-brand-500: #872DED") {
		t.Errorf("theme CSS does not map Airflow's brand color:\n%s", got)
	}
}

// A page that already carries the theme gets it once. The desktop injects it on
// its own iframe proxies as well, and whichever runs second must not stack a
// second copy on top.
func TestInjectAirflowThemeLeavesAThemedPageAlone(t *testing.T) {
	resp := newHTMLResponse(t, "<html><head></head><body></body></html>", false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	if err := InjectAirflowTheme(resp); err != nil {
		t.Fatal(err)
	}
	if err := InjectAirflowTheme(resp); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(readBody(t, resp), themeTag); n != 1 {
		t.Errorf("theme injected %d times, want once", n)
	}
}

// The theme is the proxy's own, on every page it relays, and a host's hooks see
// the themed page after it.
func TestTheProxyThemesEveryAirflowPage(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<html><head></head><body>airflow</body></html>")
	}))
	t.Cleanup(backend.Close)
	_, port, _ := strings.Cut(strings.TrimPrefix(backend.URL, "http://"), ":")

	p := NewProxy("0", NewStore(t.TempDir()))
	var hookSawTheme bool
	p.ModifyResponse = []func(*http.Response) error{func(resp *http.Response) error {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		hookSawTheme = bytes.Contains(body, []byte(themeTag))
		resp.Body = io.NopCloser(bytes.NewReader(body))
		return nil
	}}

	rec := httptest.NewRecorder()
	p.getOrCreateProxy(port).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://x.localhost/", http.NoBody))

	if !strings.Contains(rec.Body.String(), themeTag) {
		t.Errorf("relayed page is not themed: %q", rec.Body.String())
	}
	if !hookSawTheme {
		t.Error("the host's hook ran before the theme, so it cannot build on the themed page")
	}
}
